# PDF Export

Every cluster profile HTML page includes a **🖨 Print / Save as PDF** button in the header.

## Usage

1. Open a cluster profile in your browser (served or static HTML)
2. Click **🖨 Print / Save as PDF** in the top-right header
3. In the browser print dialog, select **Save as PDF** → A4

Or use the keyboard shortcut: `Ctrl+P` / `Cmd+P`

## Print layout

The `@media print` CSS automatically:

- **Hides** interactive elements: filter bar, graph SVG, detail panel, toggle buttons, search box
- **Shows** print-only summary tables:
  - Kustomizations (Name, Path, Version, Domain, DependsOn)
  - Git Sources (Name, URL, Branch, Interval)
  - ArgoCD Applications (Name, Project, Repo, Path, Revision, Namespace)
- Applies a **light-on-white** color scheme (GitHub light style)
- Sets **A4 page size** with 1.5cm margins
- Adds **page breaks** between sections

## No extra dependencies

PDF export is implemented purely via browser CSS — no Chromium, no Go PDF library, no additional binaries required.
