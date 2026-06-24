# Design

The required diagrams are stored as draw.io sources and image exports:

- [Data flow draw.io source](design/flow.drawio)
- [Data flow image](design/flow.svg)
- [ERD draw.io source](design/erd.drawio)
- [ERD image](design/erd.svg)

## Service boundaries

The web app talks only to the API gateway. The gateway coordinates four logical backend services:

- Rule service for active rule versions and publishing new versions.
- Violation service for fine calculation, snapshot capture, and violation history.
- Payment service for the mocked charge flow.
- Notification service for invoice-style events.

PostgreSQL is the only infrastructure used because the slice needs durable rules, violations, payments, users, and notification records. No broker or object storage is included because the assignment slice does not require them yet.

## Immutability guarantee

Rule changes do not affect past violations because each issued violation stores the applied rule version id, version number, and rule snapshot alongside the calculated fine. History and payment views read that stored snapshot instead of recalculating from the latest active rule.
