# Docker Fan Control

A web-based fan control application for IPMI-enabled servers. Control fans based on GPU, CPU, or drive temperatures using customizable profiles.

![License](https://img.shields.io/badge/license-BSD--2--Clause-blue.svg)

## Features

- **Multi-Vendor IPMI Support** - ASRock Rack, Dell, Supermicro with auto-detection
- **NVIDIA GPU Monitoring** - Temperature, load, memory, and power via NVML
- **System Monitoring** - CPU temps (Intel/AMD), drive temps (SMART), CPU load
- **Control Algorithms** - Linear, Step, and PID with visual curve editor
- **Multi-Zone Profiles** - Run different profiles per fan zone simultaneously
- **Real-time Dashboard** - WebSocket-based live monitoring
- **Safety Features** - Emergency temperature threshold, 100% speed on shutdown
- **Fan Identification** - Spin up individual fans for physical identification

## Quick Start

### Prerequisites

- Docker and Docker Compose
- IPMI-capable server with `/dev/ipmi0`
- NVIDIA Container Toolkit (optional, for GPU monitoring)

### Installation

```bash
git clone https://github.com/youruser/docker-fan-control.git
cd docker-fan-control

# Create .env file
cat > .env << EOF
AUTH_JWT_SECRET=$(openssl rand -hex 32)
AUTH_DEFAULT_PASSWORD=changeme
EOF

# Start services
docker-compose up -d
```

Access the web interface at `http://localhost:3000` (login: `admin` / your password).

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `IPMI_MODE` | `local` | `local` (via /dev/ipmi0) or `lan` (network) |
| `IPMI_HOST` | - | BMC IP address (LAN mode only) |
| `IPMI_USER` | - | BMC username (LAN mode only) |
| `IPMI_PASS` | - | BMC password (LAN mode only) |
| `AUTH_JWT_SECRET` | - | JWT signing secret (**required**) |
| `AUTH_DEFAULT_PASSWORD` | `admin` | Initial admin password |
| `CONTROL_INTERVAL` | `5s` | Fan control loop interval |

### Docker Compose

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
      - AUTH_JWT_SECRET=${AUTH_JWT_SECRET}
      - AUTH_DEFAULT_PASSWORD=${AUTH_DEFAULT_PASSWORD:-admin}
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

1. **Detect Fans** - Go to Fans page → Click "Detect Fans"
2. **Identify Fans** - Use "Identify" to spin up each fan and assign labels
3. **Configure Zones** - Assign fans to zones (e.g., Zone 0 = CPU, Zone 1 = Case)
4. **Create Profile** - Go to Profiles → Create a control profile:
   - Choose algorithm (Linear/Step/PID)
   - Select input sensors (GPU temp, CPU temp, etc.)
   - Assign target zones
5. **Activate** - Click "Activate" to start fan control

### Control Algorithms

| Algorithm | Use Case | Description |
|-----------|----------|-------------|
| **Linear** | General use | Fan speed scales linearly between min/max temps |
| **Step** | Quiet operation | Discrete speed steps at temperature thresholds |
| **PID** | Precision | Maintains target temperature with smooth response |

### Multi-Zone Example

Run different profiles for different fan groups:
- "CPU Cooling" → Zone 0 based on CPU temperature
- "GPU Cooling" → Zone 1 based on GPU temperature

If profiles overlap on the same zone, the **highest speed wins** for safety.

## Supported Hardware

| Vendor | Manual Mode | Per-Zone Control | Notes |
|--------|:-----------:|:----------------:|-------|
| ASRock Rack | ✅ | ✅ | ROMED8-2T tested; static RPM reporting |
| Dell PowerEdge | ✅ | ✅ | iDRAC-based servers |
| Supermicro | ✅ | ✅ | X9/X10/X11 series |
| Generic | ❌ | ❌ | Fallback for unknown boards |

The system auto-detects your motherboard at startup. Override in Settings if needed.

## Architecture

```
┌──────────────────┐     ┌──────────────────┐
│  React Frontend  │────▶│   Nginx Proxy    │
└──────────────────┘     └────────┬─────────┘
                                  │
                         ┌────────▼─────────┐
                         │   Go Backend     │
                         │  ┌────────────┐  │
                         │  │ Controller │  │
                         │  └─────┬──────┘  │
                         │        │         │
                         │  ┌─────▼──────┐  │
                         │  │  Services  │  │
                         │  └─────┬──────┘  │
                         └────────┼─────────┘
                    ┌─────────────┼─────────────┐
              ┌─────▼────┐  ┌─────▼────┐  ┌─────▼────┐
              │ ipmitool │  │   NVML   │  │  SQLite  │
              └──────────┘  └──────────┘  └──────────┘
```

## API

See [backend/api-docs.md](backend/api-docs.md) for full API documentation.

Key endpoints:
- `POST /api/fans/detect` - Detect IPMI fan sensors
- `POST /api/profiles` - Create control profile
- `GET /api/monitoring/metrics` - Get all sensor data
- `WS /ws` - Real-time updates

## Troubleshooting

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for detailed solutions.

**Common issues:**

| Problem | Solution |
|---------|----------|
| No fans detected | Check `/dev/ipmi0` exists and container is privileged |
| Fan control not working | Enable manual mode in BIOS; try different driver in Settings |
| Static RPM values | Normal for some BMCs (ASRock); duty cycle % is accurate |
| GPUs not detected | Install NVIDIA Container Toolkit; check compose GPU config |

## Development

```bash
# Backend
cd backend && go run ./cmd/server

# Frontend
cd frontend && npm install && npm run dev

# Build images
docker-compose build
```

Requirements: Go 1.22+, Node.js 18+, ipmitool

## Contributing

1. Fork the repository
2. Create a feature branch
3. Submit a pull request

### Adding Hardware Support

See [backend/internal/services/drivers/README.md](backend/internal/services/drivers/README.md) for driver implementation guide.

When contributing new hardware support, include:
- Motherboard model and BMC firmware version
- Working IPMI raw commands
- Zone layout documentation

## License

BSD 2-Clause License - see [LICENSE](LICENSE).

## Acknowledgments

- [go-nvml](https://github.com/NVIDIA/go-nvml) - NVIDIA GPU monitoring
- [gopsutil](https://github.com/shirou/gopsutil) - System metrics
- [GORM](https://gorm.io/) - Database ORM
