# Readme-from-Docx Reference

## Locale README Files (docs/readmes/)

Update Previews in all of these except `README.md` and `README_zh-CN.md` (those two are updated in Steps 2 and 3).

- README.md          — base English (Step 2)
- README_zh-CN.md    — Chinese, zh-CN images (Step 3)
- README_ar-SA.md    — Arabic
- README_bn-BD.md    — Bengali
- README_de-DE.md   — German
- README_es-ES.md   — Spanish
- README_fr-FR.md   — French
- README_hi-IN.md   — Hindi
- README_it-IT.md   — Italian
- README_ja-JP.md   — Japanese
- README_ko-KR.md   — Korean
- README_pt-BR.md   — Portuguese (Brazil)
- README_sl-SI.md   — Slovenian
- README_tr-TR.md   — Turkish
- README_vi-VN.md   — Vietnamese
- README_zh-TW.md   — Traditional Chinese

When new locale files are added (e.g. README_xx-YY.md), include them in the “Translate Previews” step with the same rule: en images, translated Previews text.

## Path Summary

| File / scope              | Image path                        |
|---------------------------|-----------------------------------|
| README.md (repo root)     | ./images/previews/en/imageN.png   |
| docs/readmes/README.md    | ../../images/previews/en/imageN.png |
| docs/readmes/README_zh-CN.md | ../../images/previews/zh-CN/imageN.png |
| All other docs/readmes/README_*.md | ../../images/previews/en/imageN.png |

## Previews Block Boundaries

- **Start**: First line of the Previews section, e.g. `## Previews` or localized equivalent (`## 功能預覽`, `## Predogledi`, etc.).
- **End**: The line **before** the next top-level heading, e.g. `## Server Mode Deployment` / `## 伺服器模式部署`. Ensure there is a blank line between the last `![](...)` and that `##`.

When doing a full replace of the Previews block, include in the new_string a trailing blank line and the next section heading so the document structure stays correct.
