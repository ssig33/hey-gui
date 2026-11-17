// Draws the app icon and writes PNGs of every size plus Contents.json into AppIcon.appiconset.
import AppKit

let outDir = URL(fileURLWithPath: CommandLine.arguments[1])

func draw(size: Int) -> Data {
    let s = CGFloat(size)
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: size, pixelsHigh: size, bitsPerSample: 8,
                               samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
                               colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)

    // Follow the macOS icon grid: an 824pt rounded square on a 1024pt canvas.
    let inset = s * 100 / 1024
    let rect = NSRect(x: inset, y: inset, width: s - inset * 2, height: s - inset * 2)
    let shape = NSBezierPath(roundedRect: rect, xRadius: rect.width * 0.225, yRadius: rect.width * 0.225)
    NSGradient(colors: [NSColor(red: 0.98, green: 0.80, blue: 0.25, alpha: 1),
                        NSColor(red: 0.95, green: 0.55, blue: 0.20, alpha: 1)])!
        .draw(in: shape, angle: -90)

    // Envelope
    let w = rect.width * 0.58, h = w * 0.68
    let env = NSRect(x: rect.midX - w / 2, y: rect.midY - h / 2 - rect.height * 0.02, width: w, height: h)
    let body = NSBezierPath(roundedRect: env, xRadius: w * 0.06, yRadius: w * 0.06)
    NSColor.white.setFill()
    body.fill()
    let flap = NSBezierPath()
    flap.move(to: NSPoint(x: env.minX + w * 0.04, y: env.maxY - h * 0.04))
    flap.line(to: NSPoint(x: env.midX, y: env.midY - h * 0.08))
    flap.line(to: NSPoint(x: env.maxX - w * 0.04, y: env.maxY - h * 0.04))
    flap.lineWidth = max(1, w * 0.045)
    flap.lineJoinStyle = .round
    flap.lineCapStyle = .round
    NSColor(red: 0.93, green: 0.55, blue: 0.20, alpha: 1).setStroke()
    flap.stroke()

    // Lines that suggest a list
    NSColor(white: 0.82, alpha: 1).setFill()
    for i in 0..<2 {
        let lw = w * (i == 0 ? 0.5 : 0.35)
        let line = NSRect(x: env.minX + w * 0.1, y: env.minY + h * (0.14 + 0.13 * CGFloat(i)), width: lw, height: max(1, h * 0.06))
        NSBezierPath(roundedRect: line, xRadius: line.height / 2, yRadius: line.height / 2).fill()
    }

    NSGraphicsContext.restoreGraphicsState()
    return rep.representation(using: .png, properties: [:])!
}

var images: [[String: String]] = []
for base in [16, 32, 128, 256, 512] {
    for scale in [1, 2] {
        let name = "icon_\(base)x\(base)\(scale == 2 ? "@2x" : "").png"
        try! draw(size: base * scale).write(to: outDir.appendingPathComponent(name))
        images.append(["idiom": "mac", "size": "\(base)x\(base)", "scale": "\(scale)x", "filename": name])
    }
}
let contents: [String: Any] = ["images": images, "info": ["author": "xcode", "version": 1]]
try! JSONSerialization.data(withJSONObject: contents, options: [.prettyPrinted, .sortedKeys])
    .write(to: outDir.appendingPathComponent("Contents.json"))
