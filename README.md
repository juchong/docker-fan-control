# Docker Fan Control

A self-hosted web app for **temperature- and load-driven fan control** on Linux
servers and workstations. It runs in Docker, watches your GPUs, CPU, drives, and
motherboard sensors, and drives your fans from profiles you define with a visual
curve editor.

It works on two very different classes of hardware:

- **Boards with no BMC** (most consumer/enthusiast boards — e.g. ASUS, MSI,
  Gigabyte) through the Linux **hwmon** interface, writing PWM duty directly to
  the Super-I/O chip (Nuvoton `nct6775`/`nct679x`, etc.).
- **BMC/IPMI servers** (ASRock Rack, Dell PowerEdge, Supermicro) through
  `ipmitool`.

![License](https://img.shields.io/badge/license-BSD--2--Clause-blue.svg)

---

## Features

- **Two control backends, auto-detected** — direct hwmon/sysfs PWM for BMC-less
  boards, or vendor IPMI for servers. The right one is selected at startup; you
  can override it in Settings.
- **Rich monitoring** — NVIDIA GPU temp/load/memory/power (NVML), CPU package
  temps (Intel/AMD), drive temps (NVMe/SATA via hwmon), motherboard/VRM/chipset temps, and
  CPU/GPU load.
- **Profiles with three algorithms** — **Linear**, **Step**, and **PID** — built
  in a sectioned editor with a live curve preview, a "you are here" marker driven
  by real sensor values, and a transition-smoothing preview.
- **Temperature *and* load inputs** — mix sensors freely; load (%) is projected
  onto the profile's curve so it combines meaningfully with temperatures.
  Aggregate multiple inputs with **max / min / average / weighted**.
- **Zones** — the active driver defines the zones. On hwmon, each PWM channel is
  its own zone (one fan header = one zone); on IPMI servers, zones match the
  vendor's fan groups. Profiles target one or more zones.
- **Live dashboard** — WebSocket-driven radial fan gauges, a temperature-vs-duty
  chart and a CPU/GPU-load-vs-duty chart (history persists across refreshes),
  per-zone grouping, dark/light themes, and mobile layout.
- **Safety** — emergency and warning temperature thresholds, safe startup modes,
  configurable shutdown behavior, sensor-loss handling with a startup grace
  window, and a warning when a fan that used to spin stops reporting RPM.
- **Auth** — JWT login, change-password, and optional reverse-proxy
  authentication for putting it behind Authelia/Authentik/etc.

---

## How it works

```
        Browser (React SPA)
              │  HTTPS / WebSocket
        ┌─────▼─────┐   nginx serves the SPA and proxies /api
        │  Frontend │
        └─────┬─────┘
              │  /api, /api/ws
        ┌─────▼─────────────────────────────────┐
        │              Go backend                │
        │  Controller ── evaluates active        │
        │      │         profiles every N sec    │
        │  ┌───▼────────┐   ┌───────────────┐    │
        │  │  Driver    │   │  Monitoring   │    │
        │  │ hwmon/IPMI │   │ NVML / SMART  │    │
        │  └───┬────────┘   └───────────────┘    │
        └──────┼─────────────────────────────────┘
        sysfs PWM  /  ipmitool            SQLite (profiles, settings, logs)
```

The **controller** runs on a fixed interval. Each cycle it reads the sensors a
profile references, aggregates them into a single value, maps that through the
profile's curve to a target duty, and applies it to the profile's zones —
honoring per-profile smoothing (transition time), a minimum run time, and
hysteresis. When several active profiles target the same zone, the higher
**priority** wins; ties go to the **higher speed**, for safety.

---

## Quick start

You need Docker + Docker Compose, and a strong `AUTH_JWT_SECRET` (the backend
**refuses to start** with the default secret when auth is enabled).

```bash
git clone https://github.com/juchong/docker-fan-control.git
cd docker-fan-control

cat > .env <<EOF
AUTH_JWT_SECRET=$(openssl rand -hex 32)
AUTH_DEFAULT_PASSWORD=changeme     # first-run admin password only
EOF

docker compose up -d --build
```

Then open the UI, sign in as `admin`, and go to **Fans → Detect Fans**.

### Choosing a backend

The two backends need different host setup. Pick the one that matches your
hardware.

#### BMC-less boards (hwmon)

Fans are controlled by writing to `/sys/class/hwmon/*/pwmN`, so the container
needs a **read-write `/sys`**, and the host must have the Super-I/O driver
loaded.

1. Load the sensor driver on the **host** and make it persistent. Which one
   depends on the Super-I/O chip:

   | Chip family | Typical boards | Module |
   |---|---|---|
   | Nuvoton `NCT67xx` (hwmon `nct6xxx`) | ASUS, MSI, many others | in-tree `nct6775` |
   | ITE `IT86xx`/`IT87xx`/`IT87952E` (hwmon `it8xxx`) | Gigabyte (often **two** chips) | in-tree `it87` for older chips; newer Gigabyte chips need the [frankcrawford/it87](https://github.com/frankcrawford/it87) fork via DKMS (`sudo ./dkms-install.sh`) — the in-tree driver either doesn't know the chip or its PWM writes are silently overridden by the EC |

   ```bash
   sudo modprobe nct6775            # or: sudo modprobe it87
   echo nct6775 | sudo tee /etc/modules-load.d/nct6775.conf
   # confirm it bound and exposes pwm files:
   ls /sys/class/hwmon/*/pwm1 2>/dev/null && sensors
   ```
2. In your compose service:
   ```yaml
   volumes:
     - /sys:/sys                 # read-write (NOT :ro) — the driver writes pwm
   security_opt:
     - apparmor=unconfined       # the default AppArmor profile blocks /sys writes
   environment:
     - HWMON_CHIP=               # optional allow-list of chip-name prefixes; empty = all
   ```

`apparmor=unconfined` is much narrower than `privileged: true`; it only lifts the
MAC layer that denies `/sys` writes.

Drive temperatures come from the same `/sys` mount, so no `/dev` access is
needed: NVMe drives are exposed by the kernel's built-in `nvme` hwmon
(Composite temperature plus the drive's own warning/critical thresholds), and
SATA/SAS drives by the `drivetemp` module — load it on the host
(`echo drivetemp | sudo tee /etc/modules-load.d/drivetemp.conf`). `smartctl` is
only used as a fallback for drives hwmon doesn't cover, and only when the
container is actually given block devices.

Every controllable chip is bound — a board with two Super-I/O chips gets all of
its headers. Channels that no profile or manual override targets **stay on the
firmware's own fan curve**; the driver only switches a channel to manual when
it first writes it, and hands it back on stop. So a CPU fan on a controllable
header keeps its BIOS curve unless you deliberately point a profile at it.

> **Gigabyte boards:** in firmware mode the ITE chips' duty readback is not the
> effective duty (the EC drives the fan through its own path), so such fans show
> as *firmware* with no duty figure. If the UI warns that *firmware is
> overriding fan writes*, the BIOS Smart Fan setting for that header must be
> handed over (set it to Manual/Full Speed), or the driver needs the fork's
> MMIO path.
>
> **Secure Boot:** an out-of-tree module must be signed (DKMS can sign with your
> MOK); on some hosts an ACPI resource conflict must be overridden
> (`ignore_resource_conflict=1` for `it87`, `acpi_enforce_resources=lax` for
> `nct6775`) before the driver binds.

#### IPMI servers

```yaml
devices:
  - /dev/ipmi0:/dev/ipmi0
environment:
  - IPMI_MODE=local            # or "lan" with IPMI_HOST/USER/PASS
```

#### NVIDIA GPU monitoring (optional, either backend)

Install the NVIDIA Container Toolkit on the host and add the GPU reservation:

```yaml
deploy:
  resources:
    reservations:
      devices:
        - driver: nvidia
          count: all
          capabilities: [gpu]
```

---

## Configuration

All configuration is environment variables. See [`env.example`](env.example).

| Variable | Default | Description |
|---|---|---|
| `AUTH_JWT_SECRET` | *(none)* | **Required.** JWT signing secret; the app won't start with the placeholder while auth is on. `openssl rand -hex 32`. |
| `AUTH_ENABLED` | `true` | Set `false` to disable login entirely (LAN-only trusted setups). |
| `AUTH_TOKEN_TTL` | `24h` | Session token lifetime. |
| `AUTH_DEFAULT_ADMIN` / `AUTH_DEFAULT_PASSWORD` | `admin` / *(empty)* | Initial admin, created on first run only. |
| `AUTH_RESET_ADMIN_PASSWORD` | `false` | Reset the admin password to `AUTH_DEFAULT_PASSWORD` on next start. |
| `AUTH_PROXY_ENABLED` | `false` | Trust an authenticating reverse proxy. |
| `AUTH_PROXY_HEADER` | `X-Forwarded-User` | Header carrying the proxied username. |
| `AUTH_PROXY_AUTO_CREATE` | `true` | Create users seen via the proxy header. |
| `HWMON_CHIP` | *(empty)* | hwmon backend: comma-separated allow-list of chip-name prefixes to bind (e.g. `it87952`); empty binds every `nct6xxx`/`it8xxx` chip with PWM channels. |
| `IPMI_MODE` | `local` | IPMI backend: `local` (`/dev/ipmi0`) or `lan`. |
| `IPMI_HOST` / `IPMI_USER` / `IPMI_PASS` | *(none)* | IPMI LAN credentials. |
| `CONTROL_INTERVAL` | `5s` | Control-loop period. |
| `TRUSTED_PROXIES` | *(none)* | CIDRs whose `X-Forwarded-For` is believed for per-client rate limiting (e.g. your reverse-proxy subnet). |
| `ALLOWED_ORIGINS` | *(same-origin)* | Extra allowed origins for CORS/WebSocket (comma-separated). |
| `LOG_LEVEL` | `info` | `trace` / `debug` / `info` / `warn` / `error`. |
| `DATA_PATH` | `/app/data` | SQLite + state directory (mount a volume here). |
| `SERVER_PORT` | `8080` | Backend HTTP port. |

---

## Using it

1. **Detect fans** — *Fans → Detect Fans* enumerates controllable channels/fans.
2. **Identify & label** — *Identify* briefly spins a fan so you can tell which is
   which, then give it a friendly label.
3. **Assign zones** — each fan card has a control-zone selector. On hwmon each
   fan is already its own zone and carries the same name as the fan (e.g.
   `it87952 fan1`); group fans by pointing a profile at several zones.
4. **Create a profile** — pick an algorithm, choose input sensors (GPU/CPU/drive/
   board temps and GPU/CPU load), set the curve, pick target zones, and tune the
   advanced options (smoothing, min-run, hysteresis, priority). Every field has
   an inline **(?)** with its meaning and valid range.
5. **Activate** — the controller takes over those zones immediately.

### Algorithms

| Algorithm | Best for | Behavior |
|---|---|---|
| **Linear** | Most setups | Duty ramps straight between a min and max temperature. |
| **Step** | Quiet builds | Discrete duty steps at temperature thresholds. |
| **PID** | Holding a target | Drives the sensor toward a setpoint with smooth correction. |

### Mixing temperature and load

You can add **load** inputs (GPU/CPU %) alongside temperatures. Load is projected
onto the profile's curve axis — `0 %` maps to the curve's start, `100 %` to its
end — so combining "how hot" and "how loaded" is meaningful. With **max**
aggregation the fans respond to whichever demands more cooling; with **weighted**
they blend on a common scale.

---

## Supported hardware

| Backend | Manual control | Per-zone | Notes |
|---|:---:|:---:|---|
| **hwmon** (Nuvoton `nct6775`, ITE `it87` via sysfs; multi-chip) | ✅ | ✅ | One zone per PWM channel on every bound chip; untargeted channels stay on the firmware curve. The path for BMC-less boards. |
| ASRock Rack (IPMI) | ✅ | ✅ | ROMED8-2T tested; some BMCs report static RPM (duty % is the accurate figure). |
| Dell PowerEdge (IPMI) | ✅ | ✅ | iDRAC-based. |
| Supermicro (IPMI) | ✅ | ✅ | X9/X10/X11. |
| Generic (IPMI) | ❌ | ❌ | Fallback; monitoring only, no control. |

Adding a board usually means a new driver — see
[`backend/internal/services/drivers/README.md`](backend/internal/services/drivers/README.md).

---

## Safety

Fan control can overheat hardware if it misbehaves, so the controller is
deliberately conservative:

- **Emergency & warning thresholds** — above the emergency temperature all
  managed fans go to full; a warning threshold logs before that.
- **Sensor loss** — the emergency trip only arms after valid readings are seen
  and only for profiles that actually use temperature inputs, with a startup
  grace window, so a load-only profile or a not-yet-ready sensor at boot doesn't
  false-trip to 100 %.
- **Startup mode** — choose what happens when the service (re)starts: `resume`
  the last speeds, or a fixed `percent` floor. Avoid `full` in production.
- **Shutdown** — `safety_on_shutdown` decides whether stopping the service parks
  fans at 100 % or leaves them under firmware control.
- **Fan-failure hint** — a fan that reported RPM earlier and later reads 0 while
  still being driven is flagged in the UI.

---

## Development

```bash
# Backend (needs Go 1.22+, CGO for NVML)
cd backend && go build ./... && go vet ./... && go test ./...

# Frontend (Node 20+)
cd frontend && npm install && npm run build   # tsc + vite
cd frontend && npm run dev                     # dev server on :3000

# Full stack
docker compose up -d --build
```

Working in this repo with an AI agent? See [`AGENTS.md`](AGENTS.md) for build/test
commands, architecture, and the invariants worth not breaking.

---

## API

Full reference: [`backend/api-docs.md`](backend/api-docs.md). Everything is under
`/api`, JWT-authenticated (except login/status/health). Highlights:

- `POST /api/fans/detect`, `PUT /api/fans/{id}` (label/zone),
  `POST /api/fans/{id}/speed` (manual override), `DELETE /api/fans/{id}/speed`
  (back to auto), `POST /api/fans/{id}/identify`.
- `POST /api/profiles`, `POST /api/profiles/{id}/activate` / `/deactivate`.
- `GET /api/monitoring/metrics`, and `GET /api/ws` for live updates.
- `POST /api/controller/start` / `/stop`.

---

## License

BSD 2-Clause — see [LICENSE](LICENSE).

## Acknowledgments

- [go-nvml](https://github.com/NVIDIA/go-nvml) — NVIDIA GPU monitoring
- [gopsutil](https://github.com/shirou/gopsutil) — system metrics
- [GORM](https://gorm.io/) — database ORM
- The Linux `nct6775` / hwmon maintainers — the sysfs PWM interface this builds on
