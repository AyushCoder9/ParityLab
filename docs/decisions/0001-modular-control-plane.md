# ADR 0001: Modular Go control plane

Status: accepted

Use one deployable Go control plane with explicit domain, application, and adapter packages before extracting services. This preserves testability and operational simplicity while retaining boundaries for webhook ingress, scenarios, analytics, and persistence.
