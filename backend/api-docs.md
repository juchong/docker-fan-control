# Docker Fan Control API Documentation

This document describes the REST API for the Docker Fan Control application.

## Base URL

`http://localhost:8080/api`

## Authentication

Most endpoints require authentication. Include the JWT token in the Authorization header:

```
Authorization: Bearer <your-token>
```

### Authentication Endpoints

#### Login

```
POST /api/auth/login
```

**Request Body:**
```json
{
  "username": "admin",
  "password": "your-password"
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

#### Get Current User

```
GET /api/auth/me
```

**Response:**
```json
{
  "id": 1,
  "username": "admin",
  "role": "admin"
}
```

## Profiles

Profiles define how fans should be controlled based on temperature/load inputs.

### List Profiles

```
GET /api/profiles
```

**Response:**
```json
[
  {
    "id": 1,
    "name": "Gaming Mode",
    "description": "High performance cooling",
    "algorithm": "linear",
    "is_active": true,
    "priority": 10,
    "zones": [0, 1],
    "zone_count": 2,
    "input_count": 2
  }
]
```

### Get Profile

```
GET /api/profiles/{id}
```

**Response:**
```json
{
  "id": 1,
  "name": "Gaming Mode",
  "description": "High performance cooling",
  "algorithm": "linear",
  "algorithm_params": {
    "min_temp": 30,
    "max_temp": 80,
    "min_speed": 30,
    "max_speed": 100,
    "input_aggregation": "or"
  },
  "is_active": true,
  "priority": 10,
  "zones": [0, 1],
  "inputs": [
    {
      "id": 1,
      "profile_id": 1,
      "input_type": "gpu_temp",
      "input_index": 0,
      "weight": 1.0
    }
  ]
}
```

### Create Profile

```
POST /api/profiles
```

**Request Body:**
```json
{
  "name": "Gaming Mode",
  "description": "High performance cooling",
  "algorithm": "linear",
  "algorithm_params": {
    "min_temp": 30,
    "max_temp": 80,
    "min_speed": 30,
    "max_speed": 100,
    "input_aggregation": "or"
  },
  "priority": 10,
  "zones": [0, 1],
  "inputs": [
    {
      "input_type": "gpu_temp",
      "input_index": 0,
      "weight": 1.0
    }
  ]
}
```

**Validation Rules:**
- `name` is required
- `algorithm` must be one of: `linear`, `step`, `pid`
- `zones` must be valid zone IDs for the current driver
- Algorithm parameters must be valid for the selected algorithm
- `priority` must be >= 0
- `transition_time` must be between 0 and 300 seconds
- `min_run_time` must be between 0 and 300 seconds
- `hysteresis` must be between 0 and 10

### Update Profile

```
PUT /api/profiles/{id}
```

**Request Body:**
```json
{
  "name": "Updated Gaming Mode",
  "priority": 15,
  "zones": [0, 1, 2]
}
```

### Delete Profile

```
DELETE /api/profiles/{id}
```

### Activate Profile

```
POST /api/profiles/{id}/activate
```

### Deactivate Profile

```
POST /api/profiles/{id}/deactivate
```

## Fans

### List Fans

```
GET /api/fans
```

**Response:**
```json
[
  {
    "id": 1,
    "ipmi_sensor_id": "FAN1",
    "label": "CPU Fan",
    "ipmi_zone": 0,
    "detected_name": "FAN1"
  }
]
```

### Detect Fans

```
POST /api/fans/detect
```

Scans IPMI for fan sensors and saves them to the database.

### Update Fan

```
PUT /api/fans/{id}
```

**Request Body:**
```json
{
  "label": "CPU Fan",
  "ipmi_zone": 0
}
```

### Identify Fan

```
POST /api/fans/{id}/identify
```

**Request Body:**
```json
{
  "duration": 5
}
```

Spins up the fan to 100% for the specified duration (seconds) to help identify it physically.

### Set Manual Fan Speed

```
POST /api/fans/{id}/speed
```

**Request Body:**
```json
{
  "percent": 75
}
```

Sets a manual override for the fan speed (0-100%). This bypasses profile-based control.

## Controller

### Start Controller

```
POST /api/controller/start
```

Starts the fan control loop.

### Stop Controller

```
POST /api/controller/stop
```

Stops the fan control loop. Optionally sets fans to 100% for safety.

## Monitoring

### Get All Metrics

```
GET /api/monitoring/metrics
```

**Response:**
```json
{
  "gpus": [
    {
      "index": 0,
      "name": "NVIDIA GeForce RTX 3090",
      "temperature": 65,
      "load": 85,
      "fan_speed": 50,
      "memory_used": 1234567890,
      "memory_total": 24296222720,
      "power_usage": 250.5,
      "power_limit": 350.0
    }
  ],
  "system": {
    "cpu_packages": [
      {
        "index": 0,
        "name": "Package id 0",
        "temperature": 55.0
      }
    ],
    "drives": [
      {
        "index": 0,
        "device": "/dev/sda",
        "model": "Samsung SSD 860 EVO 1TB",
        "temperature": 35,
        "health": "good"
      }
    ],
    "cpu_load": 45.5,
    "memory_used": 8589934592,
    "memory_total": 34359738368
  },
  "fans": [
    {
      "id": 1,
      "ipmi_sensor_id": "FAN1",
      "label": "CPU Fan",
      "ipmi_zone": 0,
      "current_rpm": 2400,
      "manual_override": false
    }
  ],
  "controller": {
    "running": true,
    "manual_mode": true,
    "active_profiles": ["Gaming Mode"],
    "active_profile_ids": [1],
    "motherboard_vendor": "ASRock Rack",
    "motherboard_model": "ROMED8-2T",
    "motherboard_driver": "asrock_romed8",
    "driver_vendor": "ASRock Rack",
    "driver_model": "ROMED8-2T",
    "driver_capabilities": {
      "supports_manual_mode": true,
      "supports_duty_cycle_reading": true,
      "supports_per_zone_control": true,
      "max_zones": 2,
      "max_fans": 16,
      "has_static_rpm_values": true
    }
  }
}
```

### Get GPU Metrics

```
GET /api/monitoring/gpus
```

### Get System Metrics

```
GET /api/monitoring/system
```

### Get Fan Status

```
GET /api/monitoring/fans
```

## Settings

### Get Settings

```
GET /api/settings
```

**Response:**
```json
{
  "ipmi_mode": "local",
  "ipmi_host": "",
  "ipmi_user": "",
  "ipmi_password": "",
  "ipmi_command_format": "auto",
  "motherboard_vendor": "ASRock Rack",
  "motherboard_model": "ROMED8-2T",
  "motherboard_driver": "asrock_romed8",
  "safety_on_shutdown": true,
  "safety_max_temp": 85,
  "safety_speed": 100,
  "warning_temp": 70,
  "warning_enabled": true,
  "control_interval": 5
}
```

### Update Settings

```
PUT /api/settings
```

**Request Body:**
```json
{
  "safety_max_temp": 90,
  "warning_temp": 75,
  "control_interval": 10,
  "zone_layout": {
    "zones": [
      {
        "id": 0,
        "name": "CPU Zone",
        "fan_indices": [0],
        "description": "CPU cooling fan",
        "is_default": false
      },
      {
        "id": 1,
        "name": "System Zone",
        "fan_indices": [1, 2, 3],
        "description": "System cooling fans",
        "is_default": true
      }
    ]
  }
}
```

**Zone Layout Configuration:**
- `zone_layout` allows custom zone configurations
- Each zone has an `id`, `name`, `fan_indices` array, and optional `description`
- `is_default` marks the primary zone for backward compatibility
- Zone IDs must be unique and within driver's `max_zones` limit
- Fan indices reference the order of fans detected by IPMI

### Test IPMI Connection

```
POST /api/settings/test-ipmi
```

Tests the IPMI connection with current settings.

### Detect Motherboard

```
POST /api/settings/detect-motherboard
```

Auto-detects motherboard vendor, model, and sets the appropriate driver.

### List Available Drivers

```
GET /api/settings/drivers
```

**Response:**
```json
[
  {
    "vendor": "ASRock Rack",
    "model": "ROMED8-2T",
    "capabilities": {
      "supports_manual_mode": true,
      "supports_duty_cycle_reading": true,
      "supports_per_zone_control": true,
      "max_zones": 2,
      "max_fans": 16,
      "has_static_rpm_values": true
    },
    "zone_layout": {
      "zones": [
        {
          "id": 0,
          "name": "CPU Zone",
          "fan_indices": [0],
          "description": "CPU cooling fan",
          "is_default": false
        },
        {
          "id": 1,
          "name": "System Zone",
          "fan_indices": [1, 2, 3, 4, 5, 6],
          "description": "System cooling fans",
          "is_default": true
        }
      ]
    }
  }
]
```

## Profile Conflict Resolution

When multiple profiles target the same zone, the system uses priority-based conflict resolution with detailed logging.

### Conflict Resolution Rules

1. **Priority-based**: Higher priority profiles take precedence
2. **Same priority**: Higher calculated speed wins
3. **Logging**: All conflicts are logged with details
4. **Alerts**: Multiple consecutive conflicts trigger warnings

### Conflict Log Examples

**Priority conflict:**
```
Profile conflict resolved: higher priority profile took control
Zone: 0, Profile ID: 2, Profile Name: "High Priority", Priority: 20, Old Priority: 10
```

**Same priority conflict:**
```
Profile conflict resolved: same priority, higher speed selected
Zone: 0, Profile ID: 2, Profile Name: "Profile B", Priority: 10, Speed: 80
```

**Lower priority skipped:**
```
Profile conflict: lower priority profile skipped
Zone: 0, Profile ID: 1, Profile Name: "Low Priority", Priority: 5, Existing Priority: 10
```

### Viewing Conflicts

Conflicts are logged as system events with category "system" and can be queried via the logs API:

```bash
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/logs?category=system&search=conflict"
```

## Logs

### List Events

```
GET /api/logs
```

**Query Parameters:**
- `level`: Filter by level (info, warning, error)
- `category`: Filter by category (fan, profile, ipmi, system, temp, auth)
- `search`: Search in message
- `start_time`: Start time (ISO 8601)
- `end_time`: End time (ISO 8601)
- `limit`: Number of items per page
- `offset`: Page offset

**Response:**
```json
{
  "events": [
    {
      "id": 1,
      "timestamp": "2024-01-01T12:34:56.789Z",
      "level": "info",
      "category": "profile",
      "message": "Profile activated",
      "details": {
        "profile_id": 1,
        "profile_name": "Gaming Mode"
      }
    }
  ],
  "total_count": 1,
  "limit": 50,
  "offset": 0
}
```

## WebSocket

Connect to `ws://localhost:8080/ws` for real-time updates.

**Message Format:**
```json
{
  "type": "metrics",
  "timestamp": "2024-01-01T12:34:56.789Z",
  "data": {
    "gpus": [...],
    "system": {...},
    "fans": [...],
    "controller": {...}
  }
}
```

## Error Responses

All error responses follow this format:

```json
{
  "error": "error message",
  "details": {
    "field": "field name",
    "message": "validation error message"
  }
}
```

Common error codes:
- `400 Bad Request`: Invalid input data
- `401 Unauthorized`: Missing or invalid authentication
- `404 Not Found`: Resource not found
- `500 Internal Server Error`: Server error

## Algorithm Parameters

### Linear Algorithm

```json
{
  "min_temp": 30,
  "max_temp": 80,
  "min_speed": 30,
  "max_speed": 100,
  "input_aggregation": "or"
}
```

- `min_temp`: Temperature at which `min_speed` is used (°C)
- `max_temp`: Temperature at which `max_speed` is used (°C)
- `min_speed`: Minimum fan speed (0-100%)
- `max_speed`: Maximum fan speed (0-100%)
- `input_aggregation`: `or` (use max value) or `and` (use min value)

### Step Algorithm

```json
{
  "steps": [
    {"temp": 30, "speed": 30},
    {"temp": 50, "speed": 50},
    {"temp": 70, "speed": 75},
    {"temp": 80, "speed": 100}
  ],
  "input_aggregation": "or"
}
```

- `steps`: Array of temperature-speed pairs, sorted by temperature
- `input_aggregation`: Same as linear algorithm

### PID Algorithm

```json
{
  "setpoint": 70,
  "kp": 2.0,
  "ki": 0.1,
  "kd": 1.0,
  "min_speed": 30,
  "max_speed": 100,
  "input_aggregation": "or"
}
```

- `setpoint`: Target temperature (°C)
- `kp`: Proportional gain
- `ki`: Integral gain
- `kd`: Derivative gain
- `min_speed`: Minimum fan speed (0-100%)
- `max_speed`: Maximum fan speed (0-100%)
- `input_aggregation`: Same as linear algorithm

## Input Types

All input types require an `input_index` to specify which sensor to use:

- `gpu_temp`: GPU temperature (e.g., GPU 0, GPU 1)
- `gpu_load`: GPU utilization percentage
- `cpu_temp`: CPU package temperature (e.g., CPU 0, CPU 1)
- `cpu_load`: Overall CPU load percentage (index ignored)
- `drive_temp`: Drive temperature (e.g., Drive 0, Drive 1)

## Troubleshooting

For detailed troubleshooting guidance, see [TROUBLESHOOTING.md](../TROUBLESHOOTING.md).
