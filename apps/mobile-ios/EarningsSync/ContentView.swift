import SwiftUI
import EarningsSyncAPI

struct ContentView: View {
    @State private var baseURL = "http://127.0.0.1:8090"
    @State private var ticker = "AAPL"
    @State private var status = "尚未加载"
    @State private var storageLine = ""
    @State private var facts: [FactRow] = []
    @State private var loading = false

    var body: some View {
        NavigationStack {
            List {
                Section("API") {
                    TextField("Base URL", text: $baseURL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    TextField("Ticker", text: $ticker)
                        .textInputAutocapitalization(.characters)
                    Button(loading ? "加载中…" : "加载事实") {
                        Task { await reload() }
                    }
                    .disabled(loading)
                }
                Section("存储并存") {
                    Text(storageLine.isEmpty ? "打开后会显示 Sheets / PG 模式" : storageLine)
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                Section("Facts (\(facts.count))") {
                    if facts.isEmpty {
                        Text(status).foregroundStyle(.secondary)
                    } else {
                        ForEach(facts) { row in
                            VStack(alignment: .leading, spacing: 2) {
                                Text("\(row.fiscalYear ?? 0)Q\(row.fiscalQuarter ?? 0) · \(row.lineItem ?? "")")
                                    .font(.subheadline.weight(.semibold))
                                Text(valueText(row))
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            .navigationTitle("earnings-sync")
            .task { await reloadMetaOnly() }
        }
    }

    private func valueText(_ row: FactRow) -> String {
        let v = row.value.map { String($0) } ?? "—"
        let u = row.unit ?? ""
        return u.isEmpty ? v : "\(v) \(u)"
    }

    private func client() -> EarningsSyncClient {
        EarningsSyncClient(baseURL: URL(string: baseURL) ?? URL(string: "http://127.0.0.1:8090")!)
    }

    @MainActor
    private func reloadMetaOnly() async {
        do {
            let meta = try await client().meta()
            applyStorage(meta.storage)
            status = "契约 \(meta.apiVersion)"
        } catch {
            status = "meta 失败：\(error.localizedDescription)"
        }
    }

    @MainActor
    private func reload() async {
        loading = true
        defer { loading = false }
        do {
            let meta = try await client().meta()
            applyStorage(meta.storage)
            let out = try await client().facts(ticker: ticker)
            facts = out.facts
            status = facts.isEmpty ? "无数据（先 dual/pg-only ingest）" : "共 \(facts.count) 条 · API \(meta.apiVersion)"
        } catch {
            facts = []
            status = error.localizedDescription
        }
    }

    private func applyStorage(_ info: StorageInfo?) {
        guard let info else {
            storageLine = "storage 未返回"
            return
        }
        let want = info.wantMode ?? "?"
        let mode = info.mode ?? "?"
        let sheets = info.sheetsKind ?? "?"
        let pg = info.pgKind ?? "?"
        storageLine = "want=\(want) · effective=\(mode) · sheets=\(sheets) · pg=\(pg)"
    }
}

#Preview {
    ContentView()
}
