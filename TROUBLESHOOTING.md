# Docker Fan Control - Troubleshooting Guide

This guide helps you diagnose and resolve common issues with the Docker Fan Control application.

## Table of Contents

- [Profile Conflicts](#profile-conflicts)
- [Zone Assignment Issues](#zone-assignment-issues)
- [IPMI Connection Problems](#ipmi-connection-problems)
- [Fan Control Not Working](#fan-control-not-working)
- [Algorithm Tuning](#algorithm-tuning)
- [Performance Issues](#performance-issues)
- [Database Issues](#database-issues)
- [Logging and Debugging](#logging-and-debugging)
- [Common Error Messages](#common-error-messages)

## Profile Conflicts

Multiple profiles can be active simultaneously, but they must control different zones. If two profiles target the same zone, the system uses priority-based conflict resolution with detailed logging.

### Symptoms

- Fans not responding as expected
- Unexpected fan speeds
- Multiple profiles showing as active but fans behave strangely
- Warning messages in logs about profile conflicts

### Diagnosis

1. **Check active profiles**: Go to the Profiles page and see which profiles are active
2. **Review zone assignments**: Check which zones each profile controls
3. **Check priorities**: Higher priority profiles override lower priority ones
4. **View conflict logs**: Check system logs for conflict resolution messages

### Resolution

1. **Assign different zones**: Ensure each profile controls a different set of zones
2. **Adjust priorities**: Set higher priority for the profile that should take precedence
3. **Deactivate conflicting profiles**: If you don't need both profiles active
4. **Review conflict logs**: Use the logs API to find specific conflict details

### Example Scenario

You have two profiles:
- "CPU Cooling" (Priority: 10) → Zone 0
- "GPU Cooling" (Priority: 10) → Zone 0

**Problem**: Both profiles target Zone 0 with the same priority

**Solution**:
- Change "GPU Cooling" priority to 5 (lower)
- OR assign "GPU Cooling" to Zone 1
- OR deactivate one profile

### Viewing Conflict Logs

Conflicts are logged with detailed information:

```bash
# View all profile conflicts
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/logs?category=system&search=conflict"

# View recent system events
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/logs?category=system&limit=50"
```

**Log message examples:**

```
Profile conflict resolved: higher priority profile took control
Zone: 0, Profile ID: 2, Profile Name: "High Priority", Priority: 20, Old Priority: 10
```

```
Profile conflict resolved: same priority, higher speed selected
Zone: 0, Profile ID: 2, Profile Name: "Profile B", Priority: 10, Speed: 80
```

```
Profile conflict: lower priority profile skipped
Zone: 0, Profile ID: 1, Profile Name: "Low Priority", Priority: 5, Existing Priority: 10
```

## Zone Assignment Issues

Zone assignments must match your motherboard's capabilities. The system now supports customizable zone layouts.

### Symptoms

- "Invalid zone" error when creating/updating profiles
- Fans not responding to profile changes
- Profile validation failures
- "Zone X does not exist in driver layout" error

### Diagnosis

1. **Check driver capabilities**: Go to Settings → Detect Motherboard
2. **Review zone layout**: Check which zones are available
3. **Verify fan assignments**: Go to Fans page to see zone assignments
4. **Check custom zone layout**: If you've configured custom zones, verify they're valid

### Resolution

1. **Use valid zones**: Only use zones that your driver supports
2. **Check zone limits**: Don't exceed `max_zones` from driver capabilities
3. **Assign fans to zones**: Ensure fans are assigned to zones on the Fans page
4. **Configure custom zones**: Use the zone layout API to customize zones
5. **Reset to default**: Remove custom zone layout to use driver defaults

### Zone Examples by Motherboard

**ASRock Rack ROMED8-2T**:
- Zone 0: FAN1 (CPU fan)
- Zone 1: FAN2-FAN7 (System fans)

**Dell PowerEdge**:
- Zones 0-7: Multiple zones depending on model

**Supermicro**:
- Zones 0-3: Typically 4 zones

### Custom Zone Layout Configuration

You can customize zone layouts via the API:

```bash
# Get current settings (includes zone layout)
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/settings"

# Update zone layout
curl -X PUT -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
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
          "fan_indices": [1, 2, 3, 4, 5],
          "description": "System cooling fans",
          "is_default": true
        }
      ]
    }
  }' \
  "http://localhost:8080/api/settings"

# Reset to driver defaults (remove zone_layout from settings)
curl -X PUT -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{}' \
  "http://localhost:8080/api/settings"
```

**Zone Layout Format:**
- `id`: Unique zone identifier (0 to max_zones-1)
- `name`: Human-readable name for the zone
- `fan_indices`: Array of fan indices (0-based, matches IPMI detection order)
- `description`: Optional description
- `is_default`: Mark as default zone for backward compatibility
- `color`: Optional color code for UI display

**Important Notes:**
- Zone IDs must be unique and within driver's `max_zones` limit
- Fan indices must reference existing fans (check Fans page)
- At least one zone should be marked as `is_default`
- Invalid zone layouts may cause profile validation to fail
- Always test after changing zone layout

## IPMI Connection Problems

IPMI is required for fan control. Connection issues prevent fan detection and control.

### Symptoms

- "No fans detected"
- "IPMI connection failed"
- Fan detection finds no fans
- Fan control commands fail

### Diagnosis

1. **Check IPMI device**: `ls -la /dev/ipmi0`
2. **Test ipmitool**: `ipmitool sdr`
3. **Check container permissions**: Ensure privileged mode
4. **Review logs**: Check backend logs for IPMI errors

### Resolution

1. **Verify hardware**: Ensure your motherboard has IPMI/BMC
2. **Check device access**: 
   ```bash
   ls -la /dev/ipmi0
   ```
3. **Test ipmitool manually**:
   ```bash
   ipmitool sdr
   ipmitool raw 0x3a 0xd7  # For ASRock
   ```
4. **Check Docker configuration**:
   ```yaml
   services:
     fan-control-backend:
       privileged: true
       devices:
         - /dev/ipmi0:/dev/ipmi0
   ```
5. **Try LAN mode**: If local mode doesn't work, configure BMC IP

### IPMI Mode Configuration

**Local Mode** (default):
- Uses `/dev/ipmi0` directly
- Requires privileged container
- Best performance

**LAN Mode**:
- Connects to BMC via network
- Use when local mode doesn't work
- Configure in Settings:
  - IPMI Host: BMC IP address
  - IPMI User: BMC username
  - IPMI Pass: BMC password

## Fan Control Not Working

Fans are detected but not responding to control commands.

### Symptoms

- Fans stay at default speed
- Fan speed changes don't take effect
- Manual mode not enabled
- Profiles active but no fan response

### Diagnosis

1. **Check manual mode**: Ensure manual mode is enabled
2. **Verify profile activation**: Check that profiles are active
3. **Test manual control**: Try setting fan speed manually
4. **Review logs**: Look for IPMI command errors
5. **Check BIOS settings**: Some motherboards require manual mode in BIOS

### Resolution

1. **Enable manual mode**:
   - Go to Settings → Detect Motherboard
   - Or manually enable via API
2. **Activate profiles**: Ensure at least one profile is active
3. **Test manual control**:
   - Go to Fans page
   - Click "Speed" on a fan
   - Set manual speed
4. **Check BIOS**:
   - Enter BIOS setup
   - Look for "Fan Control Mode" or "IPMI Fan Control"
   - Set to "Manual" or "Disabled"
5. **Verify IPMI commands**:
   - Check which command format works for your motherboard
   - Go to Settings → IPMI Command Format
   - Try different formats

### Manual Mode Troubleshooting

If manual mode doesn't enable:

1. **Try different command formats**:
   - ASRock ROMED8 (0x3a 0xd8)
   - ASRock Legacy (0x3a 0x01)
   - Dell (0x30 0x30 0x01)
   - Supermicro (0x30 0x45 0x01)

2. **Check BMC firmware**: Some older firmware versions don't support manual mode

3. **Try manual command**:
   ```bash
   ipmitool raw 0x3a 0xd8 01 01 01 01 01 01 01 00 00 00 00 00 00 00 00 00
   ```

## Algorithm Tuning

Proper algorithm tuning ensures optimal fan performance and acoustics.

### Linear Algorithm

**When to use**: Simple, predictable fan curves

**Tuning guide**:

1. **Start with defaults**:
   - Min Temp: 30°C
   - Max Temp: 80°C
   - Min Speed: 30%
   - Max Speed: 100%

2. **Adjust based on needs**:
   - For quieter operation: Increase min_temp, decrease max_temp
   - For better cooling: Decrease min_temp, increase max_temp
   - Keep min_speed above 20% to prevent fan stalls

3. **Test and monitor**:
   - Load your system
   - Monitor temperatures and fan speeds
   - Adjust until you find the right balance

### Step Algorithm

**When to use**: Discrete speed changes, good for quiet operation

**Tuning guide**:

1. **Start with 3-5 steps**:
   ```
   30°C → 30%
   50°C → 50%
   70°C → 75%
   80°C → 100%
   ```

2. **Adjust step locations**:
   - Wider steps = quieter but slower response
   - Narrower steps = more responsive but potentially noisier

3. **Ensure ascending order**: Steps must be sorted by temperature

4. **Test with load**:
   - Run benchmarks
   - Check if fans step appropriately
   - Adjust thresholds as needed

### PID Algorithm

**When to use**: Precise temperature control, maintains setpoint

**Tuning guide**:

1. **Start with conservative values**:
   - Setpoint: 70°C
   - Kp: 2.0
   - Ki: 0.1
   - Kd: 1.0
   - Min Speed: 30%
   - Max Speed: 100%

2. **Tune Kp (Proportional)**:
   - Increase for faster response
   - Too high = overshoot and oscillation
   - Start: 1.0-3.0

3. **Tune Ki (Integral)**:
   - Eliminates steady-state error
   - Too high = overshoot and oscillation
   - Should be much smaller than Kp (0.01-0.2)

4. **Tune Kd (Derivative)**:
   - Reduces overshoot and oscillation
   - Too high = noisy, erratic behavior
   - Start: 0.5-2.0

5. **Stability check**:
   - Ki << Kp (typically 10x smaller)
   - Kd < Kp
   - No sustained oscillations

### Input Aggregation

**OR Logic** (default): Uses maximum value across all inputs
- Fans respond to the hottest sensor
- Best for safety and responsiveness
- Recommended for most use cases

**AND Logic**: Uses minimum value across all inputs
- Fans only speed up when all sensors are hot
- Can be quieter but potentially less safe
- Use when you want fans to stay low unless everything is warm

## Performance Issues

The application should run smoothly, but performance issues can occur. Enhanced monitoring helps diagnose these issues.

### Symptoms

- Slow UI updates
- Fan control lag
- High CPU usage
- WebSocket disconnects
- Control loop execution time warnings

### Diagnosis

1. **Check control interval**: Too short intervals increase CPU usage
2. **Review active profiles**: Too many profiles can slow things down
3. **Monitor logs**: Look for errors or warnings
4. **Check system resources**: CPU, memory, disk I/O
5. **Review monitoring metrics**: Check for high execution times or failures

### Resolution

1. **Adjust control interval**:
   - Default: 5 seconds
   - Increase to 10 seconds if CPU is high
   - Don't go below 2 seconds
   - Check current interval: `settings.control_interval`

2. **Simplify profiles**:
   - Use fewer input sources
   - Avoid overlapping zones
   - Reduce number of active profiles

3. **Reduce monitoring load**:
   - Fewer GPU monitoring intervals
   - Disable unused sensors

4. **Check system resources**:
   - Ensure container has enough CPU/memory
   - Monitor host system performance

5. **Check for profile conflicts**:
   ```bash
   curl -H "Authorization: Bearer <token>" \
     "http://localhost:8080/api/logs?category=system&search=conflict"
   ```

   Frequent conflicts can slow down the control loop.

## Database Issues

Database corruption or migration problems can cause issues.

### Symptoms

- "no such column" errors
- Application fails to start
- Settings not saving
- Profiles not loading

### Diagnosis

1. **Check logs**: Look for database errors
2. **Verify database file**: Check if `data/fan-control.db` exists
3. **Test database connection**: Try accessing the database manually

### Resolution

1. **Backup database**:
   ```bash
   cp data/fan-control.db data/fan-control.db.backup
   ```

2. **Recreate database** (last resort):
   ```bash
   rm data/fan-control.db
   docker-compose restart fan-control-backend
   ```

3. **Restore from backup**:
   ```bash
   cp data/fan-control.db.backup data/fan-control.db
   docker-compose restart fan-control-backend
   ```

4. **Check file permissions**:
   ```bash
   chmod 644 data/fan-control.db
   chown <user>:<group> data/fan-control.db
   ```

## Logging and Debugging

### Viewing Logs

**Backend logs**:
```bash
docker-compose logs fan-control-backend
```

**Frontend logs**:
```bash
docker-compose logs fan-control-frontend
```

**Follow logs in real-time**:
```bash
docker-compose logs -f fan-control-backend
```

### Useful Log Messages

**IPMI errors**:
```
Failed to set fan speed: command not supported
```
→ Try different IPMI command format

**Profile conflicts**:
```
Profile skipped zone (lower priority)
```
→ Check profile priorities and zone assignments

**Control loop issues**:
```
Control loop execution time exceeded 1 second
```
→ Increase control interval or simplify profiles

**Emergency conditions**:
```
Emergency temperature threshold exceeded
```
→ Check temperature sensors and emergency settings

### Debugging Steps

1. **Enable debug logging**: Set `LOG_LEVEL=debug` in environment
2. **Test IPMI manually**:
   ```bash
   docker-compose exec fan-control-backend ipmitool sdr
   docker-compose exec fan-control-backend ipmitool raw 0x3a 0xd7
   ```
3. **Check active profiles**:
   ```bash
   curl -H "Authorization: Bearer <token>" http://localhost:8080/api/profiles
   ```
4. **Test fan control**:
   ```bash
   curl -X POST -H "Authorization: Bearer <token>" http://localhost:8080/api/fans/1/speed -d '{"percent": 75}'
   ```

## Common Error Messages

### "Invalid zone: zone X exceeds driver max zones (Y)"

**Cause**: Trying to use a zone ID that's higher than the driver supports

**Solution**:
- Check driver capabilities in Settings
- Use zone IDs within the supported range (0 to max_zones-1)
- For ASRock ROMED8: zones 0 and 1 only

### "Profile validation failed: min_temp must be less than max_temp"

**Cause**: Linear or step algorithm has invalid temperature range

**Solution**:
- Set min_temp lower than max_temp
- Example: min_temp=30, max_temp=80

### "Failed to enable manual fan mode"

**Cause**: IPMI command to enable manual mode failed

**Solution**:
- Try different IPMI command format
- Check if manual mode is supported by your BMC
- Some motherboards require BIOS setting changes

### "No fans detected"

**Cause**: IPMI can't find fan sensors

**Solution**:
- Verify `/dev/ipmi0` exists
- Check container has privileged access
- Test with `ipmitool sdr`
- Try LAN mode if local mode doesn't work

### "Connection refused" to WebSocket

**Cause**: WebSocket connection failed

**Solution**:
- Check if backend is running
- Verify network connectivity
- Check firewall settings
- Try refreshing the browser

### "Database migration failed"

**Cause**: Database schema migration error

**Solution**:
- Backup database
- Recreate database (last resort)
- Check for file permission issues

## Advanced Troubleshooting

### Command Discovery for Unsupported Boards

If your motherboard isn't supported, you can discover the IPMI commands:

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

### Manual IPMI Testing

Test fan speed commands manually:

**ASRock Rack ROMED8**:
```bash
# Set all fans to 50%
ipmitool raw 0x3a 0xd6 0x32 0x32 0x32 0x32 0x32 0x32 0x32 0x1e 0x1e 0x1e 0x1e 0x1e 0x1e 0x1e 0x1e 0x1e

# Enable manual mode on FAN1-7
ipmitool raw 0x3a 0xd8 0x01 0x01 0x01 0x01 0x01 0x01 0x01 0x00 0x00 0x00 0x00 0x00 0x00 0x00 0x00 0x00
```

**Dell PowerEdge**:
```bash
# Set zone 0 to 50%
ipmitool raw 0x30 0x30 0x02 0x00 0x32

# Enable manual mode
ipmitool raw 0x30 0x30 0x01 0x00
```

**Supermicro**:
```bash
# Set zone 0 to 50%
ipmitool raw 0x30 0x70 0x66 0x01 0x00 0x32

# Enable manual mode
ipmitool raw 0x30 0x45 0x01 0x00
```

### Support Information

When requesting support, please provide:

1. **System information**:
   - Motherboard model
   - BMC/IPMI firmware version
   - Operating system

2. **Configuration**:
   - IPMI mode (local/LAN)
   - IPMI command format
   - Active profiles and their settings

3. **Logs**:
   - Backend logs (last 50 lines)
   - Any error messages
   - WebSocket connection status

4. **Manual test results**:
   - `ipmitool sdr` output
   - `ipmitool mc info` output
   - Results of manual IPMI commands

## Still Having Issues?

If you've tried everything and still have problems:

1. Check the [GitHub Issues](https://github.com/youruser/docker-fan-control/issues) for similar problems
2. Create a new issue with detailed information about your setup
3. Include logs, configuration, and any error messages
4. Describe exactly what you've tried and what happened

The more information you provide, the faster we can help you resolve the issue!
