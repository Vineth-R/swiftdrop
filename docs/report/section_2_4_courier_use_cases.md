## 2.4 Courier Use Cases

Couriers operate in the field using the mobile-optimized web application to fulfill assigned deliveries, record transitions in parcel status, and capture proof of delivery. Figure 3 illustrates the courier use case diagram.

```
+-----------------------------------------------------------------------------------+
| Figure 3: Courier Use Case Diagram                                                |
|                                                                                   |
|                   +--------------------------------------------+                  |
|                   |                  SwiftDrop                 |                  |
|                   |                                            |                  |
|                   |   ( UC-D1: Log in )                        |                  |
|                   |                                            |                  |
|                   |   ( UC-D2: View assigned deliveries )      |                  |
|   +-----------+   |                                            |                  |
|   |  Courier  |---+   ( UC-D3: Update parcel status )          |                  |
|   +-----------+   |                                            |                  |
|                   |   ( UC-D4: Add proof or note )             |                  |
|                   |                                            |                  |
|                   |   ( UC-D5: Record failed delivery )        |                  |
|                   +--------------------------------------------+                  |
+-----------------------------------------------------------------------------------+
```

Table 2.4 details the primary use cases for the Courier actor and their corresponding API v1.1 endpoints.

### Table 2.4: Courier Use Cases

| ID | Use case | Description | API |
| :--- | :--- | :--- | :--- |
| `UC-D1` | Log in | Authenticate as a courier | `2.2` |
| `UC-D2` | View assigned deliveries | See active deliveries plus those finished on the selected day, with contacts and history | `6.1, 6.2` |
| `UC-D3` | Update parcel status | Move a parcel through Picked Up, In Transit, Out for Delivery and Delivered. Delivered needs the recipient name | `6.3` |
| `UC-D4` | Add proof or note | Recipient name, note or a JPEG or PNG photo up to 5 MB | `6.4` |
| `UC-D5` | Record failed delivery | Mark Failed with a mandatory reason of 5 to 255 characters | `6.3` |

### Key Operational Rules for Couriers:
1. **Ownership Constraint:** A courier can only view details and update statuses for parcels currently assigned to their user account (`parcel.current_courier_id == actor_user_id`). Any status update attempt by an unassigned courier returns `403 FORBIDDEN`.
2. **Terminal Delivery Requirements:** Marking a parcel `DELIVERED` requires entering the name of the person who accepted the delivery (`recipient_name`), while marking `FAILED` requires a descriptive explanation of 5 to 255 characters (`failure_reason`).
3. **Proof of Delivery:** Proofs may include recipient name, note, and an optional JPEG/PNG photo attachment (max 5 MB) stored securely and referenced by URI.

