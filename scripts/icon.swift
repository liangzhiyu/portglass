import AppKit

// A small vector icon: two directions through a translucent port window.
// Kept as source so the shipped bitmap can be reproduced without design tools.
let size = NSSize(width: 1024, height: 1024)
let image = NSImage(size: size)
image.lockFocus()
NSColor.clear.setFill()
NSRect(origin: .zero, size: size).fill()
let base = NSBezierPath(roundedRect: NSRect(x: 84, y: 84, width: 856, height: 856), xRadius: 198, yRadius: 198)
let shadow = NSShadow()
shadow.shadowColor = NSColor.black.withAlphaComponent(0.16)
shadow.shadowBlurRadius = 28
shadow.shadowOffset = NSSize(width: 0, height: -12)
NSGraphicsContext.saveGraphicsState()
shadow.set()
NSColor(calibratedRed: 0.28, green: 0.46, blue: 0.97, alpha: 1).setFill()
base.fill()
NSGraphicsContext.restoreGraphicsState()
NSGradient(colors: [NSColor(calibratedRed: 0.65, green: 0.81, blue: 1, alpha: 1), NSColor(calibratedRed: 0.24, green: 0.41, blue: 0.91, alpha: 1)])!.draw(in: base, angle: -65)

let plate = NSBezierPath(roundedRect: NSRect(x: 209, y: 235, width: 606, height: 554), xRadius: 112, yRadius: 112)
NSGraphicsContext.saveGraphicsState()
let innerShadow = NSShadow()
innerShadow.shadowColor = NSColor(calibratedRed: 0.08, green: 0.2, blue: 0.6, alpha: 0.24)
innerShadow.shadowBlurRadius = 30
innerShadow.shadowOffset = NSSize(width: 0, height: -22)
innerShadow.set()
NSColor.white.withAlphaComponent(0.12).setFill()
plate.fill()
NSGraphicsContext.restoreGraphicsState()
NSGradient(colors: [.white.withAlphaComponent(0.44), .white.withAlphaComponent(0.10)])!.draw(in: plate, angle: -80)
NSColor.white.withAlphaComponent(0.65).setStroke()
plate.lineWidth = 3
plate.stroke()

func arrow(_ y: CGFloat, right: Bool) {
    let path = NSBezierPath()
    path.lineWidth = 48
    path.lineCapStyle = .round
    path.lineJoinStyle = .round
    if right {
        path.move(to: NSPoint(x: 328, y: y))
        path.line(to: NSPoint(x: 692, y: y))
        path.move(to: NSPoint(x: 605, y: y + 86))
        path.line(to: NSPoint(x: 692, y: y))
        path.line(to: NSPoint(x: 605, y: y - 86))
    } else {
        path.move(to: NSPoint(x: 696, y: y))
        path.line(to: NSPoint(x: 332, y: y))
        path.move(to: NSPoint(x: 419, y: y + 86))
        path.line(to: NSPoint(x: 332, y: y))
        path.line(to: NSPoint(x: 419, y: y - 86))
    }
    NSColor.white.withAlphaComponent(right ? 0.98 : 0.78).setStroke()
    path.stroke()
}
arrow(620, right: true)
arrow(409, right: false)
image.unlockFocus()
let rep = NSBitmapImageRep(data: image.tiffRepresentation!)!
try rep.representation(using: .png, properties: [:])!.write(to: URL(fileURLWithPath: "resources/icon.png"))
