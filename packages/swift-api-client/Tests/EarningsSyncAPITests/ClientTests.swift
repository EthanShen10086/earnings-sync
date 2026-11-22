import XCTest
@testable import EarningsSyncAPI

final class ClientTests: XCTestCase {
    func testDecodeMetaStorage() throws {
        let json = """
        {
          "api_version": "2026-11-22.1",
          "service": "earnings-sync",
          "auth_mode": "bypass",
          "storage": {
            "want_mode": "dual",
            "mode": "sheets-only",
            "sheets_kind": "memory",
            "pg_kind": "none",
            "notes": ["DATABASE_URL unset"]
          }
        }
        """.data(using: .utf8)!
        let meta = try JSONDecoder().decode(MetaResponse.self, from: json)
        XCTAssertEqual(meta.apiVersion, "2026-11-22.1")
        XCTAssertEqual(meta.storage?.wantMode, "dual")
        XCTAssertEqual(meta.storage?.sheetsKind, "memory")
    }

    func testDecodeFacts() throws {
        let json = """
        {
          "facts": [
            {
              "idempotency_key": "k1",
              "ticker": "AAPL",
              "fiscal_year": 2026,
              "fiscal_quarter": 4,
              "line_item": "total_revenue",
              "value": 119.6,
              "unit": ""
            }
          ],
          "meta": { "count": 1, "ticker": "AAPL", "cache_hit": false }
        }
        """.data(using: .utf8)!
        let out = try JSONDecoder().decode(FactsResponse.self, from: json)
        XCTAssertEqual(out.facts.count, 1)
        XCTAssertEqual(out.facts[0].lineItem, "total_revenue")
        XCTAssertEqual(out.facts[0].value, 119.6)
    }
}
