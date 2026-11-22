#!/usr/bin/env sh
# Local validation for K8s manifests without a real cluster.
# Prefers: kubectl apply --dry-run=client -k .
# Fallback: Python YAML load of every manifest (no kube API).
set -eu
ROOT=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
cd "$ROOT"

fail() { echo "validate: $*" >&2; exit 1; }

if command -v kubectl >/dev/null 2>&1; then
  echo "validate: kubectl dry-run (kustomize)"
  kubectl apply --dry-run=client -k . >/dev/null
  echo "validate: ok (kubectl)"
  exit 0
fi

echo "validate: kubectl missing — YAML structural check"
python3 - <<'PY'
import sys, pathlib
try:
    import yaml
except ImportError:
    # stdlib-only: require --- separated docs with apiVersion+kind
    yaml = None

root = pathlib.Path(".")
files = sorted(p for p in root.glob("*.yaml") if p.name != "kustomization.yaml")
if not files:
    sys.exit("no manifests")

def check_doc(doc, path, idx):
    if not isinstance(doc, dict):
        raise SystemExit(f"{path} doc[{idx}]: not a mapping")
    for k in ("apiVersion", "kind", "metadata"):
        if k not in doc:
            raise SystemExit(f"{path} doc[{idx}]: missing {k}")

for path in files:
    text = path.read_text()
    if yaml is not None:
        docs = list(yaml.safe_load_all(text))
        for i, doc in enumerate(docs):
            if doc is None:
                continue
            check_doc(doc, path, i)
    else:
        chunks = [c for c in text.split("---") if c.strip()]
        for i, chunk in enumerate(chunks):
            if "apiVersion:" not in chunk or "kind:" not in chunk or "metadata:" not in chunk:
                raise SystemExit(f"{path} chunk[{i}]: missing apiVersion/kind/metadata")
    print(f"  ok {path.name}")
print("validate: ok (yaml)")
PY
