# OpenClaw Cron History Detail Design

## Goal

Improve the right-side detail panel in the OpenClaw cron history dialog so users see a clear loading state while detail data is being fetched and a visible error reason when detail loading fails.

## Scope

- Add a dedicated detail loading state for the right panel.
- Add a dedicated detail error state with readable failure reason.
- Keep the left-side history list behavior unchanged.
- Do not add page-switch animations; only provide an in-place loading animation for the detail area.

## Design

### State Model

The right panel should explicitly handle four states:

1. `loading`
2. `error`
3. `preparing`
4. `ready`

`loading` is shown while `GetRunDetail` is in flight.

`error` is shown when `GetRunDetail` fails and should render the parsed error message instead of leaving the panel blank.

`preparing` is retained for runs that still have no bound conversation.

`ready` renders the embedded assistant page as before.

### Failure Handling

Use the existing `getErrorMessage` helper to normalize Wails/bindings errors. When detail loading fails, show:

- A short title such as "加载运行明细失败"
- A secondary description
- The concrete reason string
- A retry action

This specifically covers cases like the gateway not being started.

### Race Handling

Switching history items quickly can cause older async detail requests to overwrite newer state. Guard detail responses with a monotonically increasing request sequence so only the latest request can update the right panel.

### Verification

- Run frontend type-checking with `vue-tsc`.
- Manually verify:
  - selecting a run shows a loading animation in the right panel
  - forcing `GetRunDetail` to fail shows a visible reason
  - retry re-runs detail loading
