import Foundation

struct Envelope<T: Decodable>: Decodable {
    let data: T
}

struct AuthStatus: Decodable {
    let authenticated: Bool
}

struct BoxView: Decodable {
    let postings: [Posting]
}

struct Contact: Decodable, Hashable {
    let name: String?
    let emailAddress: String?

    var display: String {
        let name = self.name ?? "", email = emailAddress ?? ""
        switch (name.isEmpty, email.isEmpty) {
        case (false, false): return "\(name) <\(email)>"
        case (false, true): return name
        case (true, false): return email
        case (true, true): return "?"
        }
    }
}

struct Posting: Decodable {
    let id: Int
    let topicId: Int?
    let name: String?
    let summary: String?
    let creator: Contact?
    let alternativeSenderName: String?
    let activeAt: String?
    let visibleEntryCount: Int?
    let appUrl: String?
    let seen: Bool?
}

struct Recipients: Decodable, Hashable {
    let to: [Contact]?
    let cc: [Contact]?
    let bcc: [Contact]?
}

struct Entry: Decodable, Identifiable, Hashable {
    let id: Int
    let createdAt: String?
    let creator: Contact?
    let alternativeSenderName: String?
    let kind: String?
    let recipients: Recipients?
    let body: String?
    let appUrl: String?
    /// The original HTML body, fetched only for HTML emails whose Markdown body is just an
    /// attachment placeholder.
    var html: String?
    /// The body converted for display.
    var rendered: RenderedBody?

    private enum CodingKeys: String, CodingKey {
        case id, createdAt, creator, alternativeSenderName, kind, recipients, body, appUrl
    }

    /// URL that opens this entry in the HEY web app.
    var webURL: URL? {
        appUrl.flatMap { URL(string: "\($0)#__entry_\(id)") }
    }

    var sender: String {
        if let alt = alternativeSenderName, !alt.isEmpty, alt != creator?.name {
            return Contact(name: alt, emailAddress: creator?.emailAddress).display
        }
        return creator?.display ?? "?"
    }

    var date: Date? { HeyDate.parse(createdAt) }
}

struct SearchMessage: Decodable {
    let alternativeSenderName: String?
    let creator: Contact?
    let appUrl: String?
}

struct SearchHit: Decodable {
    let topicId: Int
    let subject: String?
    let updatedAt: String?
    let messages: [SearchMessage]?
}

/// One row of the list, with display and filter values precomputed from a decoded posting.
struct Row: Identifiable, Hashable {
    let id: Int // topic_id
    /// The box item ID passed to `hey seen`. Nil for search results; it is looked up from the list instead.
    let postingID: Int?
    let subject: String
    let summary: String
    let sender: String
    let date: Date?
    let count: Int
    let appURL: URL?
    let unread: Bool
    let searchText: String

    init(id: Int, postingID: Int?, subject: String, summary: String, sender: String, email: String, date: Date?, count: Int, appURL: URL?, unread: Bool) {
        self.id = id
        self.postingID = postingID
        self.subject = subject
        self.summary = summary
        self.sender = sender
        self.date = date
        self.count = count
        self.appURL = appURL
        self.unread = unread
        searchText = [subject, summary, sender, email].joined(separator: "\n").lowercased()
    }

    /// Changes when the thread gets new entries; used to tell whether a cached thread is current.
    var version: String {
        "\(Int(date?.timeIntervalSince1970 ?? 0))-\(count)"
    }

    init?(posting p: Posting) {
        guard let topicID = p.topicId else { return nil }
        self.init(
            id: topicID,
            postingID: p.id,
            subject: p.name ?? String(localized: "(No subject)"),
            summary: p.summary ?? "",
            sender: p.alternativeSenderName ?? p.creator?.name ?? p.creator?.emailAddress ?? "?",
            email: p.creator?.emailAddress ?? "",
            date: HeyDate.parse(p.activeAt),
            count: p.visibleEntryCount ?? 1,
            appURL: p.appUrl.flatMap(URL.init(string:)),
            // Unread postings come back with seen = null.
            unread: p.seen != true
        )
    }

    init(hit: SearchHit) {
        let first = hit.messages?.first
        self.init(
            id: hit.topicId,
            postingID: nil,
            subject: hit.subject ?? String(localized: "(No subject)"),
            summary: "",
            sender: first?.alternativeSenderName ?? first?.creator?.name ?? first?.creator?.emailAddress ?? "?",
            email: first?.creator?.emailAddress ?? "",
            date: HeyDate.parse(hit.updatedAt),
            count: hit.messages?.count ?? 1,
            appURL: URL(string: first?.appUrl ?? "https://app.hey.com/topics/\(hit.topicId)"),
            unread: false
        )
    }
}

enum HeyDate {
    private static let fractional: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    private static let plain = ISO8601DateFormatter()

    /// `hey thread read` returns minute-precision times without a zone (e.g. 2026-11-17T02:14); read them as UTC.
    private static let minutes: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(identifier: "UTC")
        f.dateFormat = "yyyy-MM-dd'T'HH:mm"
        return f
    }()

    static func parse(_ string: String?) -> Date? {
        guard let string else { return nil }
        return fractional.date(from: string) ?? plain.date(from: string) ?? minutes.date(from: string)
    }

    private static let sameYear: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "MM/dd HH:mm"
        return f
    }()

    private static let otherYear: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "yy/MM/dd"
        return f
    }()

    private static let full: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "yyyy/MM/dd (EEE) HH:mm"
        return f
    }()

    static func short(_ date: Date?) -> String {
        guard let date else { return "" }
        let cal = Calendar.current
        let f = cal.component(.year, from: date) == cal.component(.year, from: .now) ? sameYear : otherYear
        return f.string(from: date)
    }

    static func long(_ date: Date?) -> String {
        date.map(full.string(from:)) ?? ""
    }
}
