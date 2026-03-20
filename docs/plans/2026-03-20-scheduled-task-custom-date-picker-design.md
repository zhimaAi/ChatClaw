# Scheduled Task Custom Date Picker Design

**Scope:** Replace the native expiration date picker in the scheduled task create/edit dialog with an in-app calendar popover that matches the current rounded, light, blue-accent visual language.

**Problem:** The current expiration field uses a hidden native `input[type="date"]` and forwards clicks via `showPicker()`. The visible trigger is custom-styled, but the opened calendar is rendered by the operating system, so it looks inconsistent with the rest of the scheduled task dialog and cannot be themed.

**Recommended approach:** Keep the stored value unchanged as the existing `YYYY-MM-DD` string, but swap the native picker for a custom calendar panel rendered inside `TaskFormContent.vue`. The panel should open from the current input shell, support month navigation, highlight today, highlight the selected date, let users clear the value, and close after a date is picked.

**Why this approach:** It fixes the visual inconsistency at the actual source instead of masking it, avoids backend or payload changes, and keeps the scope local to the scheduled task form.

**UI behavior:**
- Trigger keeps the current input-like appearance and opens a custom floating panel.
- Panel shows current month title, previous/next month controls, weekday headers, and a 6-row date grid.
- Selected day uses the existing dark/blue accent direction; today gets a lighter outline treatment.
- Dates outside the current month stay visible but muted so the panel feels complete and polished.
- Footer provides quick actions for `今天` and `清空`.
- Read-only mode keeps the field non-interactive and never opens the panel.

**Data and compatibility:**
- `form.expiresAtDate` remains the single source of truth.
- No backend DTO, API, or validation changes.
- Expired-state messaging remains based on the selected date string.

**Validation:**
- Run a focused TypeScript check/build for the frontend after implementation.
