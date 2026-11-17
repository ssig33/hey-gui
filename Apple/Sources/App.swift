import SwiftUI

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
}

@main
struct HeyGUIApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @State private var model = InboxModel()

    var body: some Scene {
        Window("HEY Imbox", id: "main") {
            ContentView()
                .environment(model)
                .frame(minWidth: 700, minHeight: 400)
                .background(WindowFrameAutosave(name: "MainWindow"))
        }
        .defaultSize(width: 1200, height: 800)

        Settings {
            SettingsView()
                .environment(model)
        }
    }
}

struct SettingsView: View {
    @Environment(InboxModel.self) private var model
    @AppStorage("heyPath") private var heyPath = ""

    var body: some View {
        Form {
            TextField("hey location", text: $heyPath, prompt: Text("Leave empty to search automatically"))
            Text("Example: /opt/homebrew/bin/hey")
                .font(.caption)
                .foregroundStyle(.secondary)
            Button("Reload with This Setting") {
                Task { await model.reload() }
            }
        }
        .padding()
        .frame(width: 460)
    }
}

/// Saves the window position and size to UserDefaults and restores them on the next launch.
struct WindowFrameAutosave: NSViewRepresentable {
    let name: String

    func makeNSView(context: Context) -> NSView {
        let view = NSView()
        DispatchQueue.main.async {
            guard let window = view.window else { return }
            window.setFrameUsingName(name)
            window.setFrameAutosaveName(name)
        }
        return view
    }

    func updateNSView(_ nsView: NSView, context: Context) {}
}
