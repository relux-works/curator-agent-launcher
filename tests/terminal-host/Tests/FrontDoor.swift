import Foundation
import Testing

private struct Fixture {
    let directory: URL
    let executable: URL

    init() throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        executable = URL(fileURLWithPath: try #require(ProcessInfo.processInfo.environment["LAUNCHER_UNDER_TEST"]))
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    }

    func run(_ args: [String]) throws -> (Int32, String) {
        let process = Process()
        process.executableURL = executable
        process.arguments = args
        process.currentDirectoryURL = directory
        process.environment = [
            "HOME": directory.path, "PATH": directory.path + ":/usr/bin:/bin",
            "XDG_CONFIG_HOME": directory.appendingPathComponent("config").path,
            "XDG_STATE_HOME": directory.appendingPathComponent("state").path
        ]
        let output = Pipe()
        process.standardOutput = output
        process.standardError = output
        try process.run()
        let bytes = output.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        return (process.terminationStatus, String(decoding: bytes, as: UTF8.self))
    }
}

// A valid manifest survives a refused replacement with a tmux command prefix.
@Test func registeredTerminal() throws {
    let f = try Fixture()
    defer { try? FileManager.default.removeItem(at: f.directory) }
    let manifest = f.directory.appendingPathComponent("manifest.json")
    let args = ["session-manager", "register", "--manifest", manifest.path, "--select", "--scope", "user"]
    func write(prefix: [String]) throws {
        let doc: [String: Any] = ["id":"terminal", "protocol":"tmux-cli/1", "executable":"/bin/sh",
                                  "registered_by":"fixture", "argv_prefix":prefix]
        try JSONSerialization.data(withJSONObject: doc).write(to: manifest)
    }
    try write(prefix: [])
    let (first, message) = try f.run(args)
    #expect(first == 0, Comment(rawValue: message))
    let (listed, before) = try f.run(["session-manager", "list", "--json"])
    #expect(listed == 0 && before.contains("tmux-cli/1") && before.contains(#""selected":"terminal""#))
    try write(prefix: ["-c"])
    let (rejected, reason) = try f.run(args)
    #expect(rejected != 0 && reason.contains("invalid"))
    let (_, after) = try f.run(["session-manager", "list", "--json"])
    #expect(after == before)
}

// An explicit notification choice and a unique host selector are checked before source discovery.
@Test func conversionAdmission() throws {
    let f = try Fixture()
    defer { try? FileManager.default.removeItem(at: f.directory) }
    let core = f.directory.appendingPathComponent("curator")
    try """
        #!/bin/sh
        echo '{"schema":"curator.launcher-config/1","config_schema":5,"config_path":"fixture.json","extensions":{}}'
        """.write(to: core, atomically: true, encoding: .utf8)
    try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: core.path)
    let args = ["codex", "from", "muse", "--resume", "01924444-3333-7222-8111-0123456789ac"]
    for (tail, reason) in [
        ([], "select --a2a"),
        (["--history-only","--a2a","fixture"], "--history-only requires"),
        (["--history-only","--host","terminal","--native"], "--host cannot be combined")
    ] {
        let (status, text) = try f.run(args + tail)
        #expect(status != 0 && text.contains(reason))
    }
    let (status, text) = try f.run(args + ["--history-only","--host","terminal"])
    #expect(status != 0 && text.contains("install the agent-session-transfer command"))
}

// Exercise the production RPC reader and installer with isolated command fixtures.
// Missing approval, another lane, duplicate events and the wrong workspace cannot satisfy trust.
@Test func effectiveHookTrust() throws {
    let f = try Fixture()
    defer { try? FileManager.default.removeItem(at: f.directory) }
    let env = ProcessInfo.processInfo.environment
    let driver = URL(fileURLWithPath: try #require(env["DOORBELL_UNDER_TEST"]))
    let bin = f.directory.appendingPathComponent("bin")
    let work = f.directory.appendingPathComponent("work")
    let home = f.directory.appendingPathComponent("managed")
    for dir in [bin, work, home] {
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true,
                                               attributes: [.posixPermissions: 0o700])
    }
    func script(_ name: String, _ script: String) throws -> URL {
        let path = bin.appendingPathComponent(name)
        try script.write(to: path, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: path.path)
        return path
    }
    let a2a = try script("a2a", """
        #!/bin/sh
        while [ "$#" -gt 1 ]; do
            if [ "$1" = "--settings" ]; then cp "$FIXTURE_SETTINGS" "$2"; exit $?; fi
            shift
        done
        exit 1
        """)
    let codex = try script("codex", """
        #!/bin/sh
        read -r init
        echo '{"id":1,"result":{}}'
        read -r initialized
        read -r list
        cat "$FIXTURE_REPLY"
        """)
    let settings = f.directory.appendingPathComponent("settings.json")
    let reply = f.directory.appendingPathComponent("reply.json")
    let target = home.appendingPathComponent("hooks.json")
    let actions = [("sessionStart","SessionStart","session-start"), ("userPromptSubmit","UserPromptSubmit","prompt"), ("stop","Stop","stop")]
    func command(_ action: String) -> String { "\(a2a.path) --as fixture hook \(action) --runtime codex" }
    let hooks = Dictionary(uniqueKeysWithValues: actions.map { ($0.1, [["hooks":[["type":"command", "command":command($0.2)]]]]) })
    try JSONSerialization.data(withJSONObject: ["hooks":hooks]).write(to: settings)
    func invoke(_ operation: String) throws -> Int32 {
        let process = Process()
        process.executableURL = driver
        process.currentDirectoryURL = work
        process.arguments = [operation, work.path, home.path, "fixture", codex.path]
        process.environment = ["HOME":f.directory.path, "PATH":bin.path + ":/usr/bin:/bin",
                               "FIXTURE_SETTINGS":settings.path,"FIXTURE_REPLY":reply.path]
        process.standardError = FileHandle.nullDevice
        try process.run()
        process.waitUntilExit()
        return process.terminationStatus
    }
    #expect(try invoke("install") == 0)
    let saved = try Data(contentsOf: target)
    #expect(saved == (try Data(contentsOf: settings)))
    let rows: [[String: Any]] = actions.map { ["eventName":$0.0, "handlerType":"command", "command":command($0.2),
                                              "enabled":true, "trustStatus":"trusted", "sourcePath":target.path] }
    func probe(_ hooks: [[String:Any]], cwd: String) throws -> Int32 {
        let result: [String:Any] = ["id":2, "result":["data":[["cwd":cwd,"hooks":hooks,"errors":[]]]]]
        try JSONSerialization.data(withJSONObject: result).write(to: reply)
        return try invoke("trust")
    }
    #expect(try probe(rows, cwd: work.path) == 0)
    for (key, value) in [("trustStatus", "untrusted"), ("command", command("stop").replacingOccurrences(of: "fixture hook", with: "other hook"))] {
        var changed = rows
        changed[2][key] = value
        #expect(try probe(changed, cwd: work.path) != 0)
    }
    #expect(try probe(rows + [rows[2]], cwd: work.path) != 0)
    #expect(try probe(rows, cwd: home.path) != 0)
    #expect(try Data(contentsOf: target) == saved)
}
