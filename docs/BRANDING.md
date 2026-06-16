# Branding

VinoLlama is an independent local-first desktop LLM tool.

It is not affiliated with, endorsed by, or sponsored by Intel, OpenVINO, Ollama, llama.cpp, or their maintainers.

## Logo

The current application logo is an AI-generated raster asset created for this project:

```text
desktop/frontend/src/assets/vinollama-logo.png
desktop/frontend/public/vinollama-logo.png
desktop/build/appicon.png
```

The mark is intentionally text-free. Product text such as `VinoLlama` is rendered by the UI and documentation, not embedded in the generated image.

Design constraints:

- Friendly local AI desktop application mark.
- No third-party logos or lookalike marks.
- No subscription, marketplace, or cloud imagery.
- No hidden text, watermark, or endorsement signal.
- Must remain readable at small app-icon sizes.

## UI Usage

The React desktop shell imports `src/assets/vinollama-logo.png` for the sidebar brand mark.

The Vite frontend uses `public/vinollama-logo.png` as the browser favicon.

The Wails project keeps `desktop/build/appicon.png` as the app icon source for future Wails packaging validation.

## Asset Handling

Do not replace the logo with a third-party brand asset. If a future logo is generated or edited, save it as a new versioned project asset first, review it for brand safety, then update all consuming paths intentionally.
