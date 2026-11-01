## 2.5 Dispatcher and Admin Use Cases

Dispatchers coordinate logistics operations, monitor courier availability, balance active workloads, and assign parcels to field couriers. System Administrators maintain overall platform health, configure zones, set tariff structures, manage staff accounts, and audit platform activities. Figure 4 illustrates the Dispatcher and Admin use case diagram.

```
+-----------------------------------------------------------------------------------+
| Figure 4: Dispatcher and Admin Use Case Diagram                                   |
|                                                                                   |
|                   +--------------------------------------------+                  |
|                   |                  SwiftDrop                 |                  |
|                   |                                            |                  |
|                   |   ( UC-A1: View unassigned parcels )       |                  |
|   +------------+  |                                            |                  |
|   | Dispatcher |--+-- ( UC-A2: Assign or reassign courier )    |                  |
|   +------------+  |                                            |                  |
|         ^         |   ( UC-A3: Monitor courier workload )      |                  |
|         |         |                                            |                  |
|         |         |   ( UC-A8: Search and monitor parcels )    |                  |
|         |         |                                            |                  |
|   +------------+  |   ( UC-A4: Manage users )                  |                  |
|   |   Admin    |--+                                            |                  |
|   +------------+  |   ( UC-A5: Approve merchants )             |                  |
|                   |                                            |                  |
|                   |   ( UC-A6: Manage zones and pricing )      |                  |
|                   |                                            |                  |
|                   |   ( UC-A7: View reports )                  |                  |
|                   +--------------------------------------------+                  |
+-----------------------------------------------------------------------------------+
```

Table 2.5 details the use cases for Dispatcher and Admin actors and their corresponding API v1.1 endpoints.

### Table 2.5: Dispatcher and Admin Use Cases

| ID | Use case | Description | API |
| :--- | :--- | :--- | :--- |
| `UC-A1` | View unassigned parcels | List parcels with no courier, filtered by pickup zone and date | `5.1` |
| `UC-A2` | Assign or reassign courier | Assign an active, available courier. Closed parcels cannot be assigned | `2.15, 5.2` |
| `UC-A3` | Monitor courier workload | Availability and active deliveries per courier, by status | `5.3` |
| `UC-A4` | Manage users | Create staff accounts, edit, activate and deactivate users (Admin) | `2.10 to 2.14` |
| `UC-A5` | Approve merchants | Approve, reject or suspend merchants (Admin) | `8.4 to 8.8` |
| `UC-A6` | Manage zones and pricing | Zones, base fees, weight bands and size surcharges (Admin) | `3.3 to 3.12` |
| `UC-A7` | View reports | Deliveries per day, success and failure rates, top failure reasons | `7.1, 7.2` |
| `UC-A8` | Search and monitor parcels | Search every parcel by status, courier, merchant, customer or tracking number, and watch all live updates | `5.4, 9.2` |

### Key Operational Rules for Dispatchers and Admins:
1. **Assignment Scope:** Dispatchers and Admins assign couriers to parcels via `POST /api/v1/dispatch/assign`. They do not change parcel lifecycle status directly (`status` transitions are reserved exclusively for the assigned courier via API 6.3).
2. **Parcel Eligibility:** Closed parcels (status `DELIVERED` or `FAILED`) cannot be assigned or reassigned (`409 PARCEL_CLOSED`).
3. **Active Assignment Uniqueness:** Only one active courier assignment (`is_current = TRUE`) can exist per parcel at any given time, enforced by a database partial unique index.
4. **Workload Visibility:** Dispatchers monitor real-time courier workload counts across statuses (`CREATED`, `PICKED_UP`, `IN_TRANSIT`, `OUT_FOR_DELIVERY`) to prevent delivery bottlenecks.

