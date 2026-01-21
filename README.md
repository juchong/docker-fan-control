# Docker Fan Control

A dockerized web-based fan control application for IPMI-based server systems with NVIDIA GPU monitoring support. Control your server fans based on GPU temperatures, CPU temperatures, or system load using customizable profiles.

## Features

- **IPMI Fan Control**: Direct local access via `/dev/ipmi0` or network-based IPMI over LAN
- **Multi-Vendor Support**: ASRock Rack, Dell, Supermicro with auto-detection and manual override
- **NVIDIA GPU Monitoring**: Multi-GPU temperature, load, memory, and fan speed monitoring using go-nvml
- **Comprehensive System Monitoring**:
  - Multi-CPU package temperature monitoring (Intel/AMD)
  - Drive temperatures via smartctl (HDD, SSD, NVMe)
  - CPU load and memory usage
- **Fan Profiles**: Linear, Step, and PID control algorithms with visual curve editor
- **Multiple Simultaneous Profiles**: Run different profiles for different fan zones simultaneously
  - Zone 0 can run a CPU-based profile while Zone 1 runs a GPU-based profile
  - Safety-first: if multiple profiles target the same zone, the highest speed wins
- **Flexible Input Selection**: 
  - Control fans based on any combination of GPU temps, CPU temps, drive temps, or load
  - **AND/OR Logic**: Choose between OR (respond to hottest) or AND (respond to coolest) aggregation
  - Multi-input selection with real-time sensor values
- **Auto-Detection**: Automatic fan sensor detection with manual labeling support
- **Fan Identification**: Spin up individual fans to physically identify them
- **Safety Shutdown**: Optionally set fans to 100% when controller stops to prevent overheating
- **Event Logging**: Comprehensive logging with export to CSV/JSON
- **User Authentication**: Username/password login with optional reverse proxy auth header support
- **Real-time Updates**: WebSocket-based live monitoring dashboard with collapsible sections for large sensor counts

## Supported Hardware

### Tested Motherboards

| Manufacturer | Model | BMC Firmware | Status |
|-------------|-------|--------------|--------|
| ASRock Rack | ROMED8-2T | 2.08 | ✅ Full Support |

### IPMI Vendors

| Vendor | Manual Mode | Fan Speed Control | Notes |
|--------|-------------|-------------------|-------|
| ASRock Rack (ROMED8) | `0x3a 0xd8` | `0x3a 0xd6` | 16-byte command sets all fans; software tracks per-zone speeds |
| ASRock Rack (Legacy) | N/A | `0x3a 0x01` | 8-byte, values in 64ths |
| Dell PowerEdge | `0x30 0x30 0x01` | `0x30 0x30 0x02` | Native per-zone control |
| Supermicro | `0x30 0x45 0x01` | `0x30 0x70 0x66` | Native per-zone control |

## Quick Start

### Prerequisites

- Docker and Docker Compose
- NVIDIA Container Toolkit (for GPU monitoring)
- IPMI-capable server with `/dev/ipmi0` device
- smartmontools (optional, for drive temperature monitoring)

### Installation

1. Clone the repository:

```bash
git clone https://github.com/youruser/docker-fan-control.git
cd docker-fan-control
```

2. Create a `.env` file:

```bash
# Generate a secure JWT secret
AUTH_JWT_SECRET=$(openssl rand -hex 32)

# Set your admin password
AUTH_DEFAULT_PASSWORD=your-secure-password
```

3. Start the services:

```bash
docker-compose up -d
```

4. Access the web interface at `http://localhost:3000`

5. Login with default credentials:
   - Username: `admin`
   - Password: `admin` (or your configured password)

**Important:** Change the default password after first login!

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_PORT` | `8080` | Backend server port |
| `DATA_PATH` | `/app/data` | Database and data storage path |
| `IPMI_MODE` | `local` | IPMI access mode: `local` or `lan` |
| `IPMI_HOST` | | BMC IP address (for LAN mode) |
| `IPMI_USER` | | BMC username (for LAN mode) |
| `IPMI_PASS` | | BMC password (for LAN mode) |
| `AUTH_ENABLED` | `true` | Enable authentication |
| `AUTH_JWT_SECRET` | | JWT signing secret (required for production) |
| `AUTH_TOKEN_TTL` | `24h` | Token expiration time |
| `AUTH_PROXY_ENABLED` | `false` | Enable reverse proxy auth |
| `AUTH_PROXY_HEADER` | `X-Forwarded-User` | Header for proxy auth |
| `AUTH_DEFAULT_ADMIN` | `admin` | Default admin username |
| `AUTH_DEFAULT_PASSWORD` | `admin` | Default admin password |
| `CONTROL_INTERVAL` | `5s` | Fan control loop interval |

### Settings (via Web UI)

Additional settings are configurable through the Settings page:

| Setting | Default | Description |
|---------|---------|-------------|
| IPMI Command Format | `auto` | Auto-detect or manually select: `asrock_romed8`, `asrock_legacy`, `dell`, `supermicro` |
| Safety on Shutdown | `true` | Set fans to 100% when controller stops |
| Safety Max Temp | `85°C` | Emergency full-speed temperature threshold |
| Loop Interval | `5s` | How often fan speeds are adjusted |

### Docker Compose Example

```yaml
services:
  fan-control-backend:
    build: ./backend
    privileged: true
    devices:
      - /dev/ipmi0:/dev/ipmi0
    volumes:
      - ./data:/app/data
      - /sys:/sys:ro
    environment:
      - SERVER_PORT=8080
      - IPMI_MODE=local
      - AUTH_ENABLED=true
      - AUTH_JWT_SECRET=${AUTH_JWT_SECRET}
      - AUTH_DEFAULT_PASSWORD=${AUTH_DEFAULT_PASSWORD:-admin}
      - CONTROL_INTERVAL=5s
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]

  fan-control-frontend:
    build: ./frontend
    ports:
      - "3000:80"
    depends_on:
      - fan-control-backend
```

## Usage

### 1. Detect Fans

Navigate to **Fans** page and click **Detect Fans** to scan for IPMI fan sensors.

### 2. Label Fans (Optional)

Use the **Identify** button to spin up individual fans for physical identification, then assign meaningful labels.

### 3. Create Profiles

Go to **Profiles** page and create fan control profiles:

- **Linear**: Fan speed scales linearly between min/max temperatures
  - Example: 30°C → 50%, 60°C → 100%
- **Step**: Discrete speed steps at temperature thresholds
  - Example: <40°C → 30%, 40-60°C → 60%, >60°C → 100%
- **PID**: Proportional-Integral-Derivative control for smooth, responsive control
  - Best for maintaining a target temperature

### 4. Configure Inputs

Select temperature/load sources for each profile. You can select multiple inputs and choose how they are combined.

#### Available Input Types

| Category | Input | Description |
|----------|-------|-------------|
| **Aggregate** | Max GPU Temperature | Highest temp across all GPUs |
| | Average GPU Temperature | Mean temp across all GPUs |
| | Max CPU Temperature | Highest temp across all CPU packages |
| | Max Drive Temperature | Highest temp across all drives |
| **GPU** | GPU Temperature (specific) | Temperature of a specific GPU (GPU0, GPU1, etc.) |
| | GPU Load (specific) | Utilization % of a specific GPU |
| **CPU** | CPU Temperature (specific) | Temperature of a specific CPU package |
| | CPU Load | CPU utilization percentage |
| **Drive** | Drive Temperature (specific) | Temperature of a specific drive (requires smartctl) |

#### Input Logic (AND/OR)

When multiple inputs are selected, you can choose how they are combined:

- **OR Logic (default)**: Uses the **maximum value** across all selected inputs. Fans respond to whichever sensor is hottest. Best for safety and responsiveness.
- **AND Logic**: Uses the **minimum value** across all selected inputs. Fans only speed up when all sensors are hot. Best for quieter operation when you want fans to stay low unless everything is warm.

**Example**: Selecting "Max GPU Temp" and "Max Drive Temp" with OR logic means fans respond to whichever is hotter. With AND logic, fans only respond to the cooler of the two.

### 5. Assign Fan Zones

Select which fan zones each profile controls:

- Go to the **Fans** page and assign IPMI zones to each fan (e.g., Zone 0, Zone 1)
- In the profile editor, select the zones this profile should control
- If no zones are selected, the profile controls all fans when activated

**Multiple Profiles**: You can run multiple profiles simultaneously if they control different zones. For example:
- Profile "CPU Cooling" → Zone 0 (CPU fans) based on CPU temperature
- Profile "GPU Cooling" → Zone 1 (case fans) based on GPU temperature

If multiple active profiles target the same zone, the **highest calculated speed wins** for safety.

### 6. Activate Profile

Click **Activate** to enable a profile. Multiple profiles can be active at once. The fan controller will automatically adjust fan speeds based on each profile's settings and current temperatures.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Web Browser                             │
│                   (React Frontend)                           │
└─────────────────────────┬───────────────────────────────────┘
                          │ HTTP/WebSocket
┌─────────────────────────▼───────────────────────────────────┐
│                    Nginx Proxy                               │
│              (Static files + API proxy)                      │
└─────────────────────────┬───────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────┐
│                   Go Backend                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ REST API    │  │ WebSocket   │  │ Fan Controller      │  │
│  │ (Chi)       │  │ (Gorilla)   │  │ (Control Loop)      │  │
│  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘  │
│         │                │                     │             │
│  ┌──────▼────────────────▼─────────────────────▼──────────┐ │
│  │                    Services                             │ │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌───────────┐  │ │
│  │  │ IPMI    │  │ GPU     │  │ System  │  │ Profiles  │  │ │
│  │  │ Service │  │ Service │  │ Service │  │ Service   │  │ │
│  │  └────┬────┘  └────┬────┘  └────┬────┘  └─────┬─────┘  │ │
│  └───────┼────────────┼────────────┼─────────────┼────────┘ │
└──────────┼────────────┼────────────┼─────────────┼──────────┘
           │            │            │             │
    ┌──────▼──────┐ ┌───▼───┐ ┌─────▼─────┐ ┌─────▼─────┐
    │ ipmitool    │ │ NVML  │ │ /sys      │ │ SQLite    │
    │ /dev/ipmi0  │ │ GPUs  │ │ gopsutil  │ │ Database  │
    └─────────────┘ └───────┘ └───────────┘ └───────────┘
```

## IPMI Commands Reference

### ASRock Rack ROMED8-2T (BMC 2.08+)

Discovered through reverse engineering. Uses OEM commands under NetFn `0x3a`.

**Known Limitation**: The BMC reports static RPM values (typically 800 RPM) regardless of actual fan speed. The dashboard displays both the reported RPM and the actual PWM duty cycle percentage, which accurately reflects the controlled fan speed.

#### Read Commands

| Command | Description | Response |
|---------|-------------|----------|
| `raw 0x3a 0xd7` | Get fan duty cycles | 16 bytes: duty % for each fan |
| `raw 0x3a 0xd9` | Get fan modes | 16 bytes: 0x01=manual, 0x00=auto |
| `raw 0x3a 0xda` | Get fan duty (alternate) | 16 bytes |
| `raw 0x3a 0xa7` | Get board name | ASCII string |

#### Write Commands

| Command | Description | Data Format |
|---------|-------------|-------------|
| `raw 0x3a 0xd6 <16 bytes>` | Set fan duty cycles | Direct percentage (0x00-0x64) for each fan position |
| `raw 0x3a 0xd8 <16 bytes>` | Set fan modes | 0x01=manual, 0x00=auto |

**Zone Control**: The ASRock ROMED8 sets all 16 fan speeds in a single 16-byte command. Each byte position corresponds to a fan slot. To achieve per-zone control, the software tracks the desired speed for each zone and builds the complete 16-byte command with the appropriate values for each position.

**Example: Set FAN1 (zone 0) to 55% and FAN2-7 (zone 1) to 65%**
```bash
ipmitool raw 0x3a 0xd6 0x37 0x41 0x41 0x41 0x41 0x41 0x41 0x1e 0x1e 0x1e 0x1e 0x1e 0x1e 0x1e 0x1e 0x1e
#                      ^    ^--------------------^    ^--------------------------------------------^
#                      |    FAN2-7 at 65% (0x41)     FAN8-16 at 30% (0x1e, unused)
#                      FAN1 at 55% (0x37)
```

**Example: Set all fans to 50%**
```bash
ipmitool raw 0x3a 0xd6 0x32 0x32 0x32 0x32 0x32 0x32 0x32 0x1e 0x1e 0x1e 0x1e 0x1e 0x1e 0x1e 0x1e 0x1e
```

**Example: Enable manual mode on FAN1-7**
```bash
ipmitool raw 0x3a 0xd8 0x01 0x01 0x01 0x01 0x01 0x01 0x01 0x00 0x00 0x00 0x00 0x00 0x00 0x00 0x00 0x00
```

### Dell PowerEdge

```bash
# Enable manual mode
ipmitool raw 0x30 0x30 0x01 0x00

# Disable manual mode (restore automatic)
ipmitool raw 0x30 0x30 0x01 0x01

# Set fan speed (all zones, 50% = 0x32)
ipmitool raw 0x30 0x30 0x02 0xff 0x32

# Set fan speed (specific zone)
ipmitool raw 0x30 0x30 0x02 <zone> <percent>
```

### Supermicro

```bash
# Enable manual mode
ipmitool raw 0x30 0x45 0x01 0x00

# Disable manual mode
ipmitool raw 0x30 0x45 0x01 0x01

# Set fan speed (zone, percent 0-255)
ipmitool raw 0x30 0x70 0x66 0x01 <zone> <percent>
```

### Discovering Commands for Other Boards

If your board isn't supported, you can discover the commands:

```bash
# Scan for valid OEM commands
for cmd in $(seq 0 255); do
  hex=$(printf "%02x" $cmd)
  result=$(ipmitool raw 0x3a 0x$hex 2>&1)
  if [[ ! "$result" =~ "Invalid command" ]]; then
    echo "0x$hex: $result"
  fi
done
```

Look for commands that return "Request data length invalid" - these are valid commands that need data parameters.

## API Reference

### Authentication

```
POST /api/auth/login     - Login with username/password
POST /api/auth/logout    - Logout
GET  /api/auth/me        - Get current user
```

### Fans

```
GET  /api/fans              - List all fans with current status
POST /api/fans/detect       - Detect IPMI fan sensors
PUT  /api/fans/{id}         - Update fan label/zone
POST /api/fans/{id}/identify - Spin up fan for identification
POST /api/fans/{id}/speed   - Set manual speed override
```

### Profiles

```
GET    /api/profiles              - List all profiles
POST   /api/profiles              - Create profile
GET    /api/profiles/{id}         - Get profile details
PUT    /api/profiles/{id}         - Update profile
DELETE /api/profiles/{id}         - Delete profile
POST   /api/profiles/{id}/activate   - Activate profile
POST   /api/profiles/{id}/deactivate - Deactivate profile
```

### Settings

```
GET  /api/settings    - Get all settings
PUT  /api/settings    - Update settings
```

### Logs

```
GET /api/logs         - Get event logs (supports filtering)
```

### WebSocket

Connect to `/ws` for real-time updates. Messages are JSON:

```json
{
  "type": "metrics",
  "timestamp": "2024-01-01T00:00:00Z",
  "data": {
    "gpus": [...],
    "system": {...},
    "fans": [...],
    "controller": {...}
  }
}
```

## Troubleshooting

### Fan detection finds no fans

1. Verify IPMI device exists: `ls -la /dev/ipmi0`
2. Test ipmitool: `ipmitool sdr type Fan`
3. Check container has privileged access

### Fan control commands fail

1. Check your board's BMC firmware version
2. Try the command discovery script above
3. Some boards require manual mode in BIOS first
4. Try manually selecting the IPMI command format in Settings instead of auto-detect

### Fan speeds are slow to change

The auto-detection tries multiple command formats sequentially. Go to **Settings** and manually select your board's command format to skip the detection process.

### RPM values don't change / show static values

Some BMC firmware (notably ASRock Rack) reports placeholder RPM values instead of actual tachometer readings. The duty cycle percentage shown is the accurate measure of fan speed. This is a firmware limitation, not a bug.

### WebSocket disconnects frequently

This is usually normal browser behavior during page navigation. The client automatically reconnects.

### GPUs not detected

1. Ensure NVIDIA Container Toolkit is installed
2. Verify GPUs are visible: `nvidia-smi`
3. Check container has GPU access in compose file

### Drives not detected / no temperature

1. Ensure smartmontools is installed in the container
2. Verify drives support SMART: `smartctl -i /dev/sda`
3. Check container has access to drive devices (may require privileged mode)
4. Some USB drives or RAID controllers don't expose SMART data

### CPU temperature not showing

1. Check if hwmon sensors are available: `ls /sys/class/hwmon/`
2. Look for coretemp (Intel) or k10temp/zenpower (AMD) drivers
3. Ensure `/sys` is mounted read-only in the container

### Database errors after upgrade

If you see "no such column" errors, delete the database file and restart:
```bash
rm /path/to/data/fan-control.db
docker-compose restart fan-control-backend
```

## Development

### Backend (Go)

```bash
cd backend
go mod download
go run ./cmd/server
```

Requirements:
- Go 1.22+
- CGO enabled (for SQLite)
- ipmitool installed
- smartmontools (for drive temperature monitoring)
- lm-sensors (optional, for additional temperature sources)

### Frontend (React)

```bash
cd frontend
npm install
npm run dev
```

Requirements:
- Node.js 18+
- npm or yarn

### Building Docker Images

```bash
# Build both images
docker-compose build

# Build specific image
docker-compose build fan-control-backend
docker-compose build fan-control-frontend
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Submit a pull request

### Adding Support for New Hardware

If you have a motherboard not currently supported:

1. Use the command discovery script to find valid OEM commands
2. Identify read/write commands for fan duty and mode
3. Add the commands to `backend/internal/services/ipmi.go`
4. Submit a PR with your findings

Please include:
- Motherboard model and manufacturer
- BMC/IPMI firmware version
- Working raw commands
- Any quirks or limitations

## License

BSD 2-Clause License - see [LICENSE](LICENSE) file for details.

## Acknowledgments

- [go-nvml](https://github.com/NVIDIA/go-nvml) - NVIDIA GPU monitoring
- [gopsutil](https://github.com/shirou/gopsutil) - System metrics
- [GORM](https://gorm.io/) - Database ORM
- [Chi](https://github.com/go-chi/chi) - HTTP router
- [Gorilla WebSocket](https://github.com/gorilla/websocket) - WebSocket support

## Related Projects

- [asrock-pwm-ipmi](https://git.deck.sh/shark/asrock-pwm-ipmi) - Python ASRock Rack fan control
- [Dell iDRAC fan control](https://github.com/cw1997/dell-fans-controller) - Dell-specific fan control
