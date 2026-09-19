# AGENTS.md

Orientation for AI agents working in this repo. Human overview is in
[`README.md`](README.md); this file is the "how it fits together and how not to
break it" guide.

## What this is

A Dockerized fan controller: a **Go backend** (REST + WebSocket, SQLite via GORM)
that drives fans through a pluggable driver layer (Linux **hwmon**/sysfs for
BMC-less boards, or vendor **IPMI** via `ipmitool`), plus a **React/TypeScript +
Vite** SPA served by nginx. NVIDIA GPU data comes from NVML (cgo).

## Layout

```
backend/
  cmd/server/            main(): config, logging, wiring, HTTP server
  internal/
    api/                 chi handlers, middleware, router, websocket
    services/            controller, ipmi service, driver registry, validation, system metrics, auth
      drivers/           hwmon.go (primary, BMC-less) + asrock/dell/supermicro/generic (IPMI)
    algorithms/          linear.go, step.go, pid.go, algorithm.go (AggregateInputs)
    models/              GORM models + request/response types + constants
    config/              env → Config
    database/            open, migrate, default settings, get/set
  api-docs.md            REST/WS reference
frontend/src/
  pages/                 Dashboard, Fans, Profiles, Settings, Logs, Login
  components/            common (Card/Button/Modal/Toast/HelpTip/DegradedBanner), layout, viz (RadialGauge, TimeSeriesPanel)
  hooks/                 useMonitoring, useMetricsHistory, useFanHealth, useAuth, useTheme
  services/              api.ts (REST), websocket.ts
```

## Build & test (the gate)

**Backend requires CGO** (go-nvml). No Go on the host? Run it in a container; a
clean module cache needs `go mod tidy` first:

```bash
cd backend
docker run --rm -e CGO_ENABLED=1 \
  -v fcbuild-gomod:/go/pkg/mod -v fcbuild-gocache:/root/.cache/go-build \
  -v "$PWD":/src:ro golang:1.22 \
  sh -c 'cp -a /src /build && cd /build && go mod tidy && go build ./... && go vet ./... && go test ./...'
```

**Frontend** — if there's no local `node`, build the Dockerfile's builder stage
(it runs `tsc && vite build`, which is the real type-check gate):

```bash
docker build --target builder -f frontend/Dockerfile -t fc-frontend-check frontend
```

Deploy: `docker compose up -d --build` (or a single service). Fan hardware is
live — see safety below.

## How control works

`services/controller.go` runs a ticker (`CONTROL_INTERVAL`). Each cycle, per
active profile: gather the sensors it references → `calculateInputValue`
(aggregate max/min/avg/weighted) → the profile's algorithm maps that to a duty →
apply to the profile's zones with smoothing / min-run / hysteresis. Conflicts on
a zone resolve by **priority (higher wins), then higher speed**.

Algorithm instances are cached by `type + json.Marshal(params)` so PID integral
state persists across cycles; they're pruned only from the control goroutine.

## Invariants — don't break these

- **Zones are driver-defined.** `GetZoneLayout()` is the source of truth. Zone
  IDs are stable and **owned by the driver** — for hwmon the ID is
  `chipSlot*100 + PWM channel` (slot = the chip's position in name-sorted order,
  so a single-chip board keeps IDs 1..N and a second chip gets 101..1xx), never
  a slice index, so a saved profile keeps addressing the same physical header
  across re-detects and reboots. Sensor IDs are `<chip>/fanN` (chip = hwmon name
  up to its first `_`). Validate zones by *membership* in the active driver's
  layout, never a `0..MaxZones` range. `fan.ipmi_zone` == driver zone ID.
- **Untargeted hwmon channels stay on the firmware curve.** `SetManualMode(true)`
  only opens a session; a channel goes manual on its first write and is released
  (`pwmN_enable` restored — `5` nct6775 / `2` it87) by `SetManualMode(false)`,
  `ReleaseZone`, or after `IdentifyZone`. `PerZoneFirmwareFallback` in the caps
  makes the controller skip the all-fans startup floor. Never add code that
  flips every channel to manual at start: the CPU fan may be on one.
- **Readback is only trusted on channels we commanded.** ITE chips in firmware
  mode return a stale/ENODATA `pwmN`; the driver reports duty `-1` and
  `control_mode=firmware`. After `overrideAfter` mismatching readbacks the chip
  is reported via `DriverWarnings()` → `controller.driver_warnings` → the UI banner.
- **Inputs may mix units.** Temps are °C, load is %. Load is projected onto the
  profile's curve axis in `calculateInputValue` (`profileInputAxis`) so it can be
  aggregated with temps. Keep temperature-only profiles byte-identical (only load
  inputs are projected). Mirror any change in the frontend "you are here" marker.
- **Everything is °C internally.** There is intentionally no temperature-unit
  setting (removed; see the GitHub issue). If you reintroduce °F it must convert
  only at the display/entry boundary and persist thresholds in °C.
- **GORM zero-value gotcha:** do **not** put `gorm:"default:..."` on fields where
  `false`/`0` is a meaningful value (e.g. `smooth_transition`, `min_run_time`) —
  GORM treats the zero value as "unset" and substitutes the default, making
  `false`/`0` impossible to store. Defaults are applied in the service layer.
- **Safety defaults:** the controller must honor `SafetyOnShutdown` on `Stop()`
  and apply a safe startup (`resume`/`percent` with a floor, immediate first
  cycle). Never let a redeploy strand fans at 100 %. The sensor-loss emergency
  must arm only after valid temps are seen, only for temp-using profiles, with a
  startup grace window (don't false-trip load-only profiles at boot). Board/VRM
  temps must **not** feed the emergency max (Super-I/O aux sensors read
  bogus-high or -55 °C when unconnected).
- **hwmon writes need a RW `/sys` and relaxed AppArmor** (`apparmor=unconfined`),
  not `privileged`. The driver captures each channel's original `pwmN_enable` to
  restore firmware/auto control. Gigabyte's newer ITE chips are only controllable
  through the frankcrawford/it87 fork (MMIO) — the in-tree `it87` accepts the
  write and the EC silently reverts it.
- **Security:** client IP for rate limiting comes from `TRUSTED_PROXIES`-gated
  `X-Forwarded-For` (never blind `chi/middleware.RealIP`). `X-Forwarded-User` is
  honored only when `AUTH_PROXY_ENABLED`. CORS/WS origin checks are exact-host.
  The app refuses to start on the default JWT secret with auth on.

## Testing notes

- `algorithms` and `services` unit tests are pure/near-pure. `calculateInputValue`
  and the `profileInputAxis`/`isLoadInput`/`paramFloat` helpers are exercised in
  `services/inputs_test.go`.
- The hwmon driver takes an injectable `sysfsRoot` (defaults to
  `/sys/class/hwmon`) so `hwmon_test.go` points it at a fake `/sys` temp dir.
- Driver-detection tests use a testify mock of the IPMI executor. Note ASRock's
  `CanDetect` returns false after its *first* probe errors (don't mock the
  second), and `GetActiveDriver` returns the cached driver without re-probing.

## Conventions

- Match surrounding style; keep comments explaining *why*, not *what*.
- Every backend change goes through `go build ./... && go vet ./... && go test ./...`
  before any `docker compose up --build`.
- New driver? Implement `IPMIDriver`; optionally `FanReadingProvider` for
  accurate duty. Register it in `main.go`. Update `drivers/README.md`.
