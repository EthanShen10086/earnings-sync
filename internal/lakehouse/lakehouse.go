// Package lakehouse sketches a warehouse / lakehouse path: partitioned fact
// tables plus an export job state machine. Without a real warehouse it keeps
// partitions in memory (or via orm.DB) so dual Sheets+PG demos still work.
package lakehouse

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/orm"
)

// Fact is one analytic fact destined for a partition.
type Fact struct {
	TenantID  string    `json:"tenant_id"`
	Ticker    string    `json:"ticker"`
	Period    string    `json:"period"` // e.g. 2026Q4
	LineItem  string    `json:"line_item"`
	Value     float64   `json:"value"`
	EventTime time.Time `json:"event_time"`
}

// PartitionKey is year/month (and optional tenant) for fact tables.
type PartitionKey struct {
	TenantID string `json:"tenant_id"`
	Year     int    `json:"year"`
	Month    int    `json:"month"`
}

func (p PartitionKey) String() string {
	t := p.TenantID
	if t == "" {
		t = "default"
	}
	return fmt.Sprintf("%s/%04d/%02d", t, p.Year, p.Month)
}

// ExportStatus for batch export jobs.
type ExportStatus string

const (
	ExportPending   ExportStatus = "pending"
	ExportRunning   ExportStatus = "running"
	ExportSucceeded ExportStatus = "succeeded"
	ExportFailed    ExportStatus = "failed"
	ExportCanceled  ExportStatus = "canceled"
)

// ExportJob copies a partition to an external sink (stub).
type ExportJob struct {
	ID         string       `json:"id"`
	Partition  PartitionKey `json:"partition"`
	Sink       string       `json:"sink"` // s3://… | file://… | stub
	Status     ExportStatus `json:"status"`
	Rows       int          `json:"rows"`
	Error      string       `json:"error,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
	FinishedAt time.Time    `json:"finished_at,omitempty"`
}

// Store holds partitioned facts + export jobs.
type Store struct {
	mu         sync.Mutex
	partitions map[string][]Fact
	jobs       map[string]*ExportJob
	orm        *orm.DB // optional; when set, also mirrors rows into ORM table
	now        func() time.Time
}

// NewStore creates a lakehouse stub. ormDB may be nil or Memory()/SQL.
func NewStore(ormDB *orm.DB) *Store {
	return &Store{
		partitions: map[string][]Fact{},
		jobs:       map[string]*ExportJob{},
		orm:        ormDB,
		now:        time.Now,
	}
}

// PartitionFor derives a key from event time.
func PartitionFor(tenantID string, eventTime time.Time) PartitionKey {
	if eventTime.IsZero() {
		eventTime = time.Now().UTC()
	}
	eventTime = eventTime.UTC()
	return PartitionKey{TenantID: tenantID, Year: eventTime.Year(), Month: int(eventTime.Month())}
}

// AppendFacts writes facts into their partitions (and optional ORM mirror).
func (s *Store) AppendFacts(ctx context.Context, facts []Fact) (PartitionKey, int, error) {
	if len(facts) == 0 {
		return PartitionKey{}, 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := PartitionFor(facts[0].TenantID, facts[0].EventTime)
	pk := key.String()
	for i := range facts {
		if facts[i].EventTime.IsZero() {
			facts[i].EventTime = s.now().UTC()
		}
		if facts[i].TenantID == "" {
			facts[i].TenantID = key.TenantID
		}
		// Keep first fact's partition for the batch (demo simplification).
		s.partitions[pk] = append(s.partitions[pk], facts[i])
		if s.orm != nil {
			row := orm.Row{
				"tenant_id":  facts[i].TenantID,
				"ticker":     strings.ToUpper(facts[i].Ticker),
				"period":     facts[i].Period,
				"line_item":  facts[i].LineItem,
				"value":      facts[i].Value,
				"event_time": facts[i].EventTime.UTC(),
				"partition":  pk,
				"year_month": fmt.Sprintf("%04d-%02d", key.Year, key.Month),
			}
			if err := s.orm.WithContext(ctx).Tenant(facts[i].TenantID).Create("lakehouse_facts", row); err != nil {
				return key, 0, err
			}
		}
	}
	return key, len(facts), nil
}

// ListPartition returns facts for a key.
func (s *Store) ListPartition(key PartitionKey) []Fact {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.partitions[key.String()]
	out := make([]Fact, len(src))
	copy(out, src)
	return out
}

// StartExport enqueues an export job in pending.
func (s *Store) StartExport(id string, key PartitionKey, sink string) (*ExportJob, error) {
	if id == "" {
		return nil, fmt.Errorf("lakehouse: export id required")
	}
	if sink == "" {
		sink = "stub://null"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[id]; ok {
		return nil, fmt.Errorf("lakehouse: export %q exists", id)
	}
	now := s.now().UTC()
	job := &ExportJob{
		ID:        id,
		Partition: key,
		Sink:      sink,
		Status:    ExportPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.jobs[id] = job
	return cloneJob(job), nil
}

// RunExport advances pending → running → succeeded|failed (stub copies row count).
func (s *Store) RunExport(id string) (*ExportJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, fmt.Errorf("lakehouse: export %q not found", id)
	}
	if job.Status != ExportPending && job.Status != ExportFailed {
		return nil, fmt.Errorf("lakehouse: export %q is %s", id, job.Status)
	}
	now := s.now().UTC()
	job.Status = ExportRunning
	job.UpdatedAt = now
	rows := len(s.partitions[job.Partition.String()])
	job.Rows = rows
	// Stub "I/O": empty partition is still success with 0 rows.
	job.Status = ExportSucceeded
	job.FinishedAt = s.now().UTC()
	job.UpdatedAt = job.FinishedAt
	return cloneJob(job), nil
}

// FailExport marks a job failed (for drill / negative tests).
func (s *Store) FailExport(id, reason string) (*ExportJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, fmt.Errorf("lakehouse: export %q not found", id)
	}
	job.Status = ExportFailed
	job.Error = reason
	job.UpdatedAt = s.now().UTC()
	return cloneJob(job), nil
}

// GetExport returns a job.
func (s *Store) GetExport(id string) (*ExportJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	return cloneJob(job), true
}

// PartitionCount returns how many partitions exist.
func (s *Store) PartitionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.partitions)
}

func cloneJob(j *ExportJob) *ExportJob {
	cp := *j
	return &cp
}
