# NOC Demo Scenario: From Alert to Resolution in 60 Seconds

## Goal
Demonstrate how NetPulse handles a real incident with suppression, drill-down, and closure in one flow.

## Step-by-step

1. **Open Dashboard (T+0s)**  
   Login to Web and show Global Health Score + Active Incident Feed.

2. **Trigger a controlled fault (T+10s)**  
   Simulate one access switch uplink outage (or disable a monitored port).

3. **Observe alert behavior (T+20s)**  
   In incident feed:
   - Core/root outage appears as critical
   - Downstream devices show related/suppressed context (not duplicated storm)

4. **Quick-Peek validation (T+30s)**  
   Click the affected device row to open Quick-Peek drawer/sheet:
   - CPU/Mem chart is visible immediately
   - Port list is available
   - Open the impacted port traffic chart

5. **Maintenance mode for controlled work (T+40s)**  
   Toggle Maintenance Mode ON for the affected device to prevent noisy alerts while work is ongoing.

6. **Recovery and confirmation (T+50s)**  
   Restore the port/link. Confirm:
   - Device returns online
   - Health score improves
   - Incident feed records recovery transition

7. **Audit closeout (T+60s)**  
   Open audit logs and verify timeline:
   - User action entries
   - Maintenance toggle record
   - Alert and recovery event chain

## Demo Success Criteria

- No alert storm for downstream devices when upstream is root cause
- Quick-Peek opens in one step with immediate operational context
- Maintenance mode suppresses alert noise but preserves data collection
- End-to-end trace is available in audit logs
