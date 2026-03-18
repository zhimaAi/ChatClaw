---
name: readme-from-docx
description: Update README and docs/readmes from Word docx (English and Chinese). Use when the user provides docx files to update feature previews, wants to sync README with docx content, or asks to extract docx images and update readmes.
---

# Update README from Docx

Use this skill when the user provides one or two docx files (e.g. English + Chinese) and wants to update the project README and all localized readmes in `docs/readmes` with new feature previews, copy, and images.

## Prerequisites

- **Docx location**: Prefer putting source docx in `think_docs/` (gitignored). Example names: `readme-英文.docx`, `readme-中文.docx`.
- **Git**: Ensure working tree is clean or changes are reviewable before bulk edits.

## Workflow Overview

1. **Extract images** from each docx into the correct preview folders.
2. **Update English copy and structure** in root README and `docs/readmes/README.md`.
3. **Update Chinese (zh-CN)** in `docs/readmes/README_zh-CN.md` with Chinese copy and zh-CN images.
4. **Translate Previews only** in all other locale READMEs; keep using **English** images.

## Step 1: Extract Images from Docx

- **Full extraction**: Extract **all** images from each docx (do not extract only a subset).
- **Paths**:
  - English docx → `images/previews/en/` (e.g. `image1.png`, `image2.png`, …).
  - Chinese docx → `images/previews/zh-CN/` (same naming if 1:1; if count differs, keep a clear mapping).
- **Method**: Unzip the docx (it is a ZIP), read `word/media/`, copy/rename to `images/previews/<en|zh-CN>/imageN.png` in document order. Or use a small script with `python-docx` / unzip to iterate and save.

After extraction, ensure no duplicate filenames and that image order matches the intended section order in the README.

## Step 2: Update English README (Root + docs/readmes)

- **Files**: `README.md` (root), `docs/readmes/README.md`.
- **Image paths**:
  - Root: `./images/previews/en/imageN.png`
  - Under docs/readmes: `../../images/previews/en/imageN.png`
- **Content**: Rewrite the **Previews** section (from `## Previews` or `## Previews`-equivalent until the next `## …` e.g. Server Mode Deployment) to match the docx:
  - Add every feature block from the docx (e.g. AI Chatbot, PPT Quick Generate, Skill Manager, MCP, Sandbox Mode, Memory, Shared Team Knowledge Base, Knowledge Base, Rich IM, Scheduled Tasks, Text Selection, Smart Sidebar, One Question Multiple Answers, One-Click Launcher Ball).
  - Each block: `### Title`, one short paragraph, then `![](path/imageN.png)`.
  - Preserve a **fixed mapping** of section ↔ image number (e.g. image1 → first section, image3 → second, image5 → third, …) so the same numbering can be reused in all locale files.
- **Formatting**: Ensure a blank line between the last preview image and the next `##` heading (avoid `image18.png)## Server Mode` on one line).

## Step 3: Update Chinese (zh-CN) README

- **File**: `docs/readmes/README_zh-CN.md`.
- **Content**: Same section order and structure as the English Previews, but **Chinese** titles and body text from the Chinese docx.
- **Images**: Use `../../images/previews/zh-CN/` (not en). Image filenames should align with the same logical section (image1 → first section, etc.); if zh-CN docx has different image count, map by section order.
- **Section title**: Use the localized heading (e.g. `## 功能預覽` or `## 功能预览`).

## Step 4: Translate Previews in All Other Locale READMEs

- **Scope**: All `docs/readmes/README_<locale>.md` **except** `README.md` (base EN) and `README_zh-CN.md`.
- **Locales** (examples): ar-SA, bn-BD, de-DE, es-ES, fr-FR, hi-IN, it-IT, ja-JP, ko-KR, pt-BR, sl-SI, tr-TR, vi-VN, zh-TW. Add any new locale files that exist under `docs/readmes/`.
- **Rule**:
  - **Images**: Always keep `../../images/previews/en/imageN.png` (same as English).
  - **Text**: Translate **only** the Previews block: section title (e.g. "Previews" → localized "Predogledi", "Önizlemeler", etc.), each `###` title, and each paragraph. Preserve the same order and the same image path per section.
- **Pitfall**: If the file has a “sticky” line like `![](../../images/previews/en/image18.png)## 伺服器模式部署`, replace it so the image is followed by a blank line and then `## 伺服器模式部署` on the next line.
- **Reference**: Use the current `docs/readmes/README.md` as the canonical list of sections and image numbers when translating.

## Section ↔ Image Mapping (Canonical)

Keep this mapping consistent across all readmes. Adjust only when the English README is intentionally changed.

| Section (EN)                         | Image file        |
|--------------------------------------|-------------------|
| AI Chatbot Assistant                 | image1.png        |
| PPT Quick Generate                   | image3.png        |
| Skill Manager                        | image5.png        |
| MCP: Unlimited Capability Extensions | image6.png        |
| Sandbox Mode: Double Protection      | image8.png        |
| Memory: More Natural, Smarter…       | image9.png        |
| Shared Team Knowledge Base           | image10.png       |
| Knowledge Base \| Document Vector…   | image11.png       |
| Rich IM Channel Integrations         | image12.png       |
| Scheduled Tasks                      | image13.png       |
| Text Selection for Instant Q&A      | image14.png, image15.png |
| Smart Sidebar                        | image16.png       |
| One Question, Multiple Answers       | image17.png       |
| One-Click Launcher Ball              | image18.png       |

When adding new features from a new docx, append new sections and new image numbers (e.g. image19, image20) and update this table in the skill or in a reference file.

## Checklist Before Finishing

- [ ] All images extracted from both docx into `images/previews/en/` and `images/previews/zh-CN/` (full set).
- [ ] Root `README.md` and `docs/readmes/README.md` have identical Previews structure and EN copy; root uses `./images/previews/en/`, docs use `../../images/previews/en/`.
- [ ] `README_zh-CN.md` uses Chinese copy and `../../images/previews/zh-CN/`.
- [ ] Every other locale README has Previews translated and still uses `../../images/previews/en/`; no “sticky” `image18.png)##` line.
- [ ] No extra subsection only in one locale (e.g. zh-TW had an extra “雙模式切換” with image3, which broke mapping; remove or align so each image number maps to one section globally).

## Optional: Reference File

For a list of all locale files and the exact Previews block template, see [reference.md](reference.md) in this skill folder.
