# ADR-014: Retain Account, Payment, and Ledger as one Banking Core

Status: Accepted

Context: Identity and Notification now run as independently deployable services. Banking Account, Payment, and Ledger have clear module boundaries but share a serializable PostgreSQL financial unit of work.

Problem: Decide whether the remaining Banking modules should be extracted into separate services solely because the architecture has become distributed elsewhere.

Decision Drivers:

- preserve atomic debit/credit posting, account locking, payment lifecycle and outbox insertion;
- keep failures diagnosable and operations manageable for a single developer;
- extract only where an independent business/data/failure boundary creates material learning or product value.

Options Considered:

1. Extract Account, Payment, and Ledger into independent services now.
2. Extract one of them first and introduce a distributed transaction/saga for booking.
3. Retain one deployable Banking Core with explicit internal module boundaries.

Selected Decision: Option 3. Account, Payment, and Ledger remain one Banking Core. They retain local serializable transactions for funds checks, deterministic account locks, balanced ledger posting, cached balance updates, payment-state transitions, audit data and transactional outbox insertion. Their application boundaries remain enforced inside the executable so an extraction can be reassessed later.

Positive Consequences:

- no distributed transaction or compensation logic is needed for financial booking;
- the debit/credit and idempotency invariants remain directly testable against one database transaction;
- deployment and incident handling remain proportionate to this educational project.

Negative Consequences:

- Banking Core is still the largest deployable and cannot scale Account/Payment/Ledger independently;
- module boundaries must be maintained deliberately to avoid a second monolith inside the process.

Risks:

- an unbounded Banking Core could become difficult to change;
- future requirements may need independent availability, backups or compliance isolation.

Mitigations:

- architecture tests, module-owned application behavior and the data-ownership role boundary remain required;
- Phase 9 fitness functions protect booking and recovery behavior;
- the decision matrix in `docs/architecture/final-service-extraction-decision.md` states concrete revisit triggers.

Revisit Conditions: Reconsider a module extraction only when it has independent data ownership, a versioned external contract, a tolerable consistency model, rehearsed migration/rollback, observable failure handling, and a concrete learning or product requirement that outweighs loss of local ACID semantics.
