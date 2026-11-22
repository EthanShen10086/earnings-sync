import Foundation

public struct StorageInfo: Decodable, Sendable {
    public let wantMode: String?
    public let mode: String?
    public let failPolicy: String?
    public let sheetsKind: String?
    public let pgKind: String?
    public let notes: [String]?

    enum CodingKeys: String, CodingKey {
        case wantMode = "want_mode"
        case mode
        case failPolicy = "fail_policy"
        case sheetsKind = "sheets_kind"
        case pgKind = "pg_kind"
        case notes
    }
}

public struct MetaResponse: Decodable, Sendable {
    public let apiVersion: String
    public let service: String
    public let authMode: String?
    public let storage: StorageInfo?

    enum CodingKeys: String, CodingKey {
        case apiVersion = "api_version"
        case service
        case authMode = "auth_mode"
        case storage
    }
}

public struct FactRow: Decodable, Sendable, Identifiable {
    public var id: String { idempotencyKey ?? "\(ticker ?? "")-\(lineItem ?? "")-\(fiscalYear ?? 0)-\(fiscalQuarter ?? 0)" }
    public let idempotencyKey: String?
    public let ticker: String?
    public let fiscalYear: Int?
    public let fiscalQuarter: Int?
    public let lineItem: String?
    public let value: Double?
    public let unit: String?

    enum CodingKeys: String, CodingKey {
        case idempotencyKey = "idempotency_key"
        case ticker
        case fiscalYear = "fiscal_year"
        case fiscalQuarter = "fiscal_quarter"
        case lineItem = "line_item"
        case value
        case unit
    }
}

public struct FactsResponse: Decodable, Sendable {
    public let facts: [FactRow]
    public let meta: [String: JSONValue]?
}

/// Minimal JSON value for open meta maps.
public enum JSONValue: Decodable, Sendable {
    case string(String)
    case number(Double)
    case bool(Bool)
    case object([String: JSONValue])
    case array([JSONValue])
    case null

    public init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null; return }
        if let b = try? c.decode(Bool.self) { self = .bool(b); return }
        if let n = try? c.decode(Double.self) { self = .number(n); return }
        if let s = try? c.decode(String.self) { self = .string(s); return }
        if let a = try? c.decode([JSONValue].self) { self = .array(a); return }
        if let o = try? c.decode([String: JSONValue].self) { self = .object(o); return }
        self = .null
    }
}

public struct EarningsSyncClient: Sendable {
    public var baseURL: URL
    public var bearerToken: String?
    public var session: URLSession

    public init(baseURL: URL = URL(string: "http://127.0.0.1:8090")!, bearerToken: String? = nil, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.bearerToken = bearerToken
        self.session = session
    }

    public func meta() async throws -> MetaResponse {
        try await get(path: "/v1/meta")
    }

    public func facts(ticker: String) async throws -> FactsResponse {
        let encoded = ticker.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? ticker
        return try await get(path: "/v1/facts?ticker=\(encoded.uppercased())")
    }

    private func get<T: Decodable>(path: String) async throws -> T {
        var req = URLRequest(url: baseURL.appendingPathComponent(String(path.dropFirst())))
        // appendingPathComponent mangles query; build manually
        req = URLRequest(url: URL(string: baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/")) + path)!)
        req.setValue("application/json", forHTTPHeaderField: "Accept")
        if let bearerToken, !bearerToken.isEmpty {
            req.setValue("Bearer \(bearerToken)", forHTTPHeaderField: "Authorization")
        }
        let (data, resp) = try await session.data(for: req)
        guard let http = resp as? HTTPURLResponse else {
            throw URLError(.badServerResponse)
        }
        guard (200..<300).contains(http.statusCode) else {
            let body = String(data: data, encoding: .utf8) ?? ""
            throw NSError(domain: "EarningsSyncAPI", code: http.statusCode, userInfo: [NSLocalizedDescriptionKey: body])
        }
        return try JSONDecoder().decode(T.self, from: data)
    }
}
