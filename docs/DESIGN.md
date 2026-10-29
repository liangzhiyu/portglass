# PortGlass interface

The interface was revised after the first version felt flat and overly boxed in. The new layout moves port and interface controls into a floating capture bar, leaves navigation in a narrow sidebar, and gives requests and their payloads a consistent reading hierarchy.

## GitHub references

- [MyGo Glass](https://github.com/egoist/mygo/blob/v0.7.2/docs/plugins/glass.md): the implementation actually used. The material draws blur, lens refraction and a lit rim through MyGo shaders. macOS uses Metal; the headless screenshot renderer has a CPU path. This is MyGo's reproduction of the material, not an invocation of SwiftUI `glassEffect`.
- [kean/Pulse](https://github.com/kean/Pulse): reference for the network inspector's separation of capture/navigation, request selection and readable details. No source or images were copied into the application.
- [SohrabZ/swiftui-macos-app](https://github.com/SohrabZ/swiftui-macos-app): reference for light/dark shell consistency and limiting glass to interface chrome. Its screenshots and documentation were inspected; no source or images were copied into the application.
- [Lucide](https://github.com/lucide-icons/lucide): embedded icon assets, pinned to the commit documented in THIRD_PARTY.md.

## Material and layout

- Glass belongs on controls: capture bar, primary action, selected navigation and direction selection. Response text is on a solid, readable surface.
- The opaque in-window color field provides a predictable backdrop for refraction. Mixing the shader's transparent render target with OS vibrancy produced a gray veil in the light appearance, so the window does not layer those two systems.
- Borders, spacing, typography, muted metadata and code colors share a light/dark palette. Content color is reserved for method, status, selection and the primary action.
- The compact layout keeps port selection, capture, search, export, format selection and the code editor visible at 1100 × 680.
- The interface uses real buttons and radio groups with keyboard focus and accessibility labels. Increase Contrast and Reduce Transparency select solid controls; Reduce Motion is honored by MyGo's control animations.
- No decorative loop animation runs while idle. The framework redraws after input or new traffic. Incoming capture events are batched before reaching the interface.

## Verification

Wide and compact light/dark layouts are rendered by `TestVisualLayouts`. Interaction checks cover protocol navigation, body/header/raw selection, copying, search, theme selection, invalid ports and compact control bounds. A native window was also inspected with macOS accessibility and screenshots. Metal frame logging confirmed the GPU rendering path; the approximately 1 ms second static frame is an observation, not a sustained frame-rate or heavy-traffic benchmark.
