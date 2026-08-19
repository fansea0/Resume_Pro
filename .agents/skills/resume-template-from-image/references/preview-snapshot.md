# Preview Snapshot Generation

End-to-end recipe for producing the static PNG cover required by step 7 of `SKILL.md`. The goal is byte-for-byte identical output to the user's real export: drive the actual `Renderer`, rasterize its DOM with the same path the app already uses (`frontend/src/utils/exportResumePng.ts`), and never hand-draw a substitute.

## Approach

A reusable Node script renders every registered template with the fixed `createSampleResumeDocument()` sample and saves one PNG per template. Two viable shapes — pick one and commit the script alongside the assets.

### Shape A — headless browser over a snapshot route (preferred)

1. Add a small dev-only route (e.g. `/__snapshot`) that mounts every registered template with `createSampleResumeDocument()` and exposes a helper that returns the DOM node for a given template id.
2. In `frontend/scripts/generate-template-previews.mjs`:
   - boot `vite` (or `vite preview` against an existing build) and wait for the URL to respond,
   - launch puppeteer / playwright with `headless: 'new'` and a viewport sized to A4 portrait,
   - for each entry in `templateRegistry`: ask the page for the rendered node by id, call the in-page helper that wraps `exportResumePng` (or `html2canvas(element, { backgroundColor: '#ffffff', scale: 2, useCORS: true })`), and write the resulting PNG to `frontend/public/template-previews/<id>.png`,
   - accept `--only=<id>` so iterating on a single template does not rerun the whole batch.
3. The in-page helper should:
   - wait for `document.fonts.ready` and a `requestAnimationFrame` tick before capturing,
   - force the node's width to A4 (≈ 794px at 96dpi or 210mm) and let it grow naturally,
   - never reach into `localStorage` or any user-saved document — the only data source is the imported sample.

### Shape B — static built page + screenshot

1. Build a dedicated HTML entry that imports the registry and renders one paper node per template, each given a unique `data-template-id` attribute.
2. `vite build` a separate bundle, serve it with any static server.
3. Drive puppeteer / playwright against `http://localhost:<port>/<entry>.html`, `page.locator('[data-template-id="<id>"]').screenshot({ type: 'png' })` for each id.
4. Skip html2canvas here — the browser-native screenshot already matches the rendered DOM pixel-for-pixel.

## Why real rendering beats hand-drawn mockups

Hand-drawn SVG previews drift from the renderer on every change: custom font metrics, line heights, the `break-inside: avoid` behaviour, chip padding, badge SVG icons, all of it. A user comparing the gallery image to their actual export immediately notices the gap and loses trust. The same `Renderer` + `html2canvas` pipeline that powers the user-facing export is the only honest source of the cover.

## Constraints carried over from SKILL.md

- Source data: `createSampleResumeDocument()` only. No user data, no source-image personal data, no network fetches.
- Output: full A4 portrait, white background, scale 2 rasterization.
- Filename: `frontend/public/template-previews/<id>.png`. The id must equal the registry id so the test in `frontend/src/templates/registry.test.tsx` keeps passing.
- Determinism: pin the script's puppeteer / playwright version in `frontend/package.json` so cover regeneration across machines does not introduce subtle rendering drift. The committed sample document should also be stable — change it deliberately, never by accident.
- Reusability: the script must work for every existing template without per-template special cases. If a template needs an extra wait condition (custom font, async data), push that into the template's renderer so the snapshot script stays uniform.

## Verification

After regenerating covers:

```bash
cd frontend && npm test -- --run src/templates/registry.test.tsx && npm run build
```

The registry test asserts each `previewImage` resolves to `/template-previews/<id>.png`; the build step catches typos in the new id. Then open the gallery in the running app and confirm each card's image matches what a fresh export of the same sample document produces.
