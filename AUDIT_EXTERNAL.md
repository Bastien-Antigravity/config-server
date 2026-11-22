Deep technical review — config-server
Repository: Bastien-Antigravity/config-server · Branch: develop · Focus: configuration merging, precedence, atomic updates, client registration, broadcasts, persistence and concurrency

This repository is particularly interesting because it is more than a configuration server: it is effectively the central state-distribution mechanism for the entire microservice ecosystem.

I see a good architectural foundation, especially the Copy-On-Write store, atomic pointer swaps, and per-client mailboxes.

However, I have already identified some areas where the implementation deserves more scrutiny. In particular:
1. The distinction between a configuration update and a full configuration replacement.
2. Concurrent broadcasts and client disconnections.
3. What happens when a client misses a configuration update.
4. Whether the persistence worker can lose a dirty-state notification.
5. Whether the in-memory configuration and the persisted configuration can diverge.
These are more consequential than simple code-style issues because they affect the consistency of every service consuming this configuration.
1. Overall architecture
The repository has three principal responsibilities:
- Maintain the current configuration state.
- Distribute configuration changes to connected services.
- Persist configuration changes to disk.
There are also several interfaces: TCP, gRPC, REST and Telegram.
Configuration producers

TCP · REST · gRPC · Telegram · local management


Core / ConfigController

Request processing · validation · merge operations


Store — atomic.Pointer + Copy-On-Write

Current configuration snapshot

Persistence worker

JSON · debounce · disk


Broadcast manager

Registry · mailboxes · TCP




Connected microservices

Live configuration synchronization


The design is sensible: business logic is separated from network adapters, and the store provides a central state authority.

The architecture documentation also explicitly describes atomic configuration snapshots and per-client mailboxes.
GitHub+2

2. Configuration merging: how it works
This is the first area I would investigate carefully.

The internal configuration model is:
type ConfigMap map[string]map[string]string


In other words:
ConfigMap
  ├── database
  │     ├── host = 127.0.0.1
  │     └── port = 5432
  ├── redis
  │     ├── host = localhost
  │     └── port = 6379
  └── trading
        ├── enabled = true
        └── mode = production


The store provides two important operations:
- Replace(): replaces the entire configuration.
- UpdateAtomic(): applies a modification to a deep copy and swaps the resulting snapshot.
The latter is especially important.
A. Copy-On-Write merging
The implementation follows this sequence:
func (s *Store) UpdateAtomic(
    modificationFn func(sandbox ConfigMap) (ConfigMap, error),
) error {

    s.mu.Lock()
    defer s.mu.Unlock()

    sandbox := DeepCopy(s.Get())

    result, err := modificationFn(sandbox)
    if err != nil {
        return err
    }

    s.config.Store(&result)
    return nil
}


This is a good implementation pattern.

Imagine the current state:
database:
  host: localhost
  port: "5432"

redis:
  host: localhost
  port: "6379"


A client sends:
database:
  host: db.internal


The intended result of a partial merge is:
database:
  host: db.internal
  port: "5432"

redis:
  host: localhost
  port: "6379"


The original state remains unchanged while the modification is being prepared.

Only when the modification succeeds does the store publish the new snapshot.

Good design
 This avoids readers seeing partially modified configuration.
B. Why DeepCopy matters
The DeepCopy() implementation correctly copies both levels of the nested map.

This is essential because Go maps are reference types.

A shallow copy would create a new outer map while retaining references to the original inner maps.

That would allow modifications to leak into the currently published configuration before the atomic swap.

The current deep-copy implementation avoids that problem.
GitHub

C. A subtle issue: who owns the new snapshot?
There is one further detail in UpdateAtomic() worth improving.

The method publishes result directly:
s.config.Store(&result)


The modification callback receives a sandbox, but it can technically return a different map, or retain a reference to the returned map.

If that reference is subsequently modified outside the store, the immutability guarantee is broken.

I would enforce ownership at the store boundary:
result, err := modificationFn(sandbox)
if err != nil {
    return err
}

committed := DeepCopy(result)
s.config.Store(&committed)


This adds an allocation on writes, but configuration updates should be relatively infrequent compared with reads.

Alternatively, document and enforce a strict ownership-transfer contract for the callback.

My preference: make the store responsible for immutability rather than relying on every caller to respect it.
3. The distinction between merging and replacing
This is probably the most important semantic distinction in the whole repository.

The system has two different operations:
PUT_SYNC — partial update

Existing configuration is retained, and supplied values are applied.

Example:
Current:  database.host = localhost
          database.port = 5432

Incoming: database.host = db.internal

Result:   database.host = db.internal
          database.port = 5432


Merge semantics

FULL_REFRESH — complete replacement

The client's existing live configuration is replaced by a complete snapshot.

Example:
Current:  database.host
          database.port
          redis.host

Incoming: database.host

Result:   database.host


Replacement semantics

Why is this important?

Suppose you delete a configuration key on the server.

A normal merge broadcast cannot communicate deletion simply by omitting that key.

The client would retain its old value.

This is precisely why the repository introduced FULL_REFRESH.

The architecture and TODO documentation acknowledge this distinction.
GitHub+1

My recommendation: make the semantics explicit
I would establish three separate operations:
Operation	Meaning
PATCH	Merge supplied keys into existing state
REPLACE	Replace the entire configuration snapshot
DELETE	Explicitly remove a key or section

Internally, all three should use the same atomic commit mechanism.

This avoids having different merging implementations in TCP, REST, gRPC and the storage layer.

It also makes it much easier to reason about client synchronization.
4. The broadcast mechanism: a significant concurrency issue
This is where I found something particularly important.

The server uses:
listeners atomic.Pointer[map[string]*clientMailbox]


This is a clever technique.

Instead of locking the map every time a broadcast occurs, the server creates a new map when listeners change and atomically swaps its pointer.

Readers can then iterate over an immutable snapshot.
A. How registration works
When a service connects:
1. The handshake provides its identity.
2. The server derives a client name using the service name and advertised address.
3. A dedicated mailbox is created.
4. The mailbox is registered in the listener map.
5. A registry update is broadcast.
For example:
database-10.0.0.5:3306
trading-10.0.0.8:3306
analytics-10.0.0.9:3306


Each client receives its own outgoing channel:
send: make(chan []byte, 64)


This is a good isolation mechanism.

A slow client should not block all other clients.
B. The critical race: closing a mailbox while broadcasting
Consider the following two goroutines:
Goroutine A — Broadcast
ptr := s.listeners.Load()

for name, mailbox := range *ptr {
    select {
    case mailbox.send <- bytes:
    default:
    }
}



Goroutine B — Disconnect
s.listenersLock.Lock()

// Remove from new listener map
s.listeners.Store(&next)

close(mailbox.send)

s.listenersLock.Unlock()



Race condition
The broadcaster may already hold a reference to the old listener map.

Removing the client from the new map does not invalidate the old snapshot.

Therefore, Goroutine A can attempt:
mailbox.send <- bytes


after Goroutine B has executed:
close(mailbox.send)


Result: panic: send on closed channel.

This is a real concurrency hazard in the inspected implementation, not merely a theoretical concern.
GitHub+1

C. How I would solve it
My preferred solution is to make the mailbox responsible for its own lifecycle.

Instead of allowing the listener manager to close a channel while broadcasts may still reference it, introduce a mailbox abstraction:
type ClientMailbox struct {
    send chan []byte
    done chan struct{}

    once sync.Once
}


And expose:
func (m *ClientMailbox) Close() {
    m.once.Do(func() {
        close(m.done)
    })
}


The writer loop becomes:
for {
    select {
    case <-mailbox.done:
        return

    case msg := <-mailbox.send:
        if _, err := sock.Write(msg); err != nil {
            return
        }
    }
}


The broadcast operation also checks the lifecycle.

However, there is an important nuance: simply adding sync.Once to the close operation is not enough. It prevents double-close, but does not prevent a concurrent send to the closed send channel.

The simplest robust solution: never close the data channel. Use a separate done channel for shutdown and keep mailbox ownership of its lifecycle.

This is simpler than coordinating every broadcaster with every disconnect.

5. Another important issue: broadcast message loss
The broadcast implementation currently uses a non-blocking send:
select {
case mailbox.send <- bytes:
default:
    s.Logger.Warning("Client %s mailbox full. Dropping message.", name)
}


The mailbox has a capacity of 64 messages.

This protects the server from slow consumers.

But consider the following:
Server configuration version 100
       |
       | Broadcast update A
       v
Client receives update A

Server configuration version 101
       |
       | Broadcast update B
       v
Mailbox full → update B dropped

Server configuration version 102
       |
       | Broadcast update C
       v
Client receives update C


The client now has version 100 or an intermediate state, while the server has version 102.

There is no guarantee that the client will ever discover the missing update.

This is particularly dangerous if updates are incremental merges.
My recommendation: versioned configuration snapshots
Introduce a monotonically increasing revision:
type ConfigSnapshot struct {
    Revision uint64
    Data     ConfigMap
}


Each committed configuration update increments the revision.

A client receiving revision 105 after revision 103 knows that something was missed.

It can request a full refresh.
Proposed synchronization protocol
1. Server commits revision 104

   Atomic store update
2. Broadcast revision 104

   Client receives the update
3. Client detects a gap

   Expected 104, received 105
4. Client requests FULL_REFRESH

   Retrieves the latest complete snapshot
5. Client installs revision 105

   State converges again

I would also coalesce pending configuration notifications.

If a client is slow, sending it 20 successive configuration snapshots is wasteful. It generally needs the latest snapshot, not every intermediate state.

A single pending refresh notification is often sufficient.

This is a major reliability improvement without requiring a sophisticated distributed message broker.
6. Persistence: atomic writes are good, but dirty-state management needs improvement
I inspected src/store/persistence.go.

There is actually good work here.

The implementation uses:
1. JSON serialization.
2. Temporary file creation in the destination directory.
3. Write to temporary file.
4. Sync() on the temporary file.
5. Close.
6. Atomic rename.
7. Directory synchronization.
This is substantially better than directly overwriting config_store.json.
Good implementation
The temporary file is created in the same directory as the destination, which is important for atomic rename behavior.

GitHub
A. The problem: dirty flag is cleared before saving succeeds
The persistence worker currently does:
if s.dirty.Swap(false) {
    if err := s.Persistence.Save(s.Store.Get()); err != nil {
        s.Logger.Error("Background persistence failed: %v", err)
    }
}


There is a subtle failure scenario.
Failure sequence
1. Configuration changes. dirty = true.
2. Worker executes dirty.Swap(false).
3. Disk write fails because of a temporary I/O error.
4. The worker logs the error but does not restore dirty.
5. No additional configuration changes occur.
Persistence may never retry

The in-memory state is correct, but the disk state is stale.

This is a particularly important issue because the whole purpose of the persistence worker is to guarantee that configuration changes eventually reach disk.
B. Proposed correction
At minimum:
if s.dirty.Swap(false) {
    snapshot := s.Store.Get()

    if err := s.Persistence.Save(snapshot); err != nil {
        s.dirty.Store(true)
        s.Logger.Error("Persistence failed: %v", err)
    }
}


However, this basic correction has a concurrency nuance.

Imagine another update occurs while the save is running. Blindly restoring dirty = true is safe in the sense that it triggers another write, but it does not distinguish the version that failed from the version that has already been saved.

I would go one step further and use a revision-based persistence mechanism.
type ConfigSnapshot struct {
    Revision uint64
    Data     ConfigMap
}


The persistence worker remembers the last successfully saved revision.
Current revision: 105
Persisted revision: 103

Save snapshot 105

Success:
Persisted revision = 105

Failure:
Persisted revision remains 103
Retry later


This removes the ambiguity surrounding dirty flags.
C. A second issue: ReloadConfig silently ignores persistence errors
The current implementation contains:
if persisted, err := s.Persistence.Load(); err == nil && persisted != nil {
    s.Store.Replace(persisted)
}


If loading fails because the JSON file is corrupted, unreadable or inaccessible, the error is silently ignored.

The function can then log:
Config reloaded successfully


and broadcast the existing state.

That is misleading.

I would make reload behavior explicit:
- File does not exist: apply the documented initial-state policy.
- File is valid: restore persisted configuration.
- File is corrupted: return an error and preserve the previous in-memory state.
- File cannot be read: return an error and preserve the previous state.
A failed reload should never be reported as successful.

GitHub+1
D. Durability qualification
The persistence implementation ignores errors from directory synchronization:
if df, err := os.Open(dir); err == nil {
    _ = df.Sync()
    _ = df.Close()
}


For normal configuration management this might be an acceptable compromise, but it does not justify an unconditional durability guarantee.

I would propagate synchronization failures where the operating system supports the operation, and explicitly document platform-specific behavior.

7. Client registration and identity management
The connection handler builds a client identifier:
clientName := fmt.Sprintf("%s-%s", name, endpoint)


This is intended to distinguish multiple instances of the same service.

For example:
trading-10.0.0.10:3306
trading-10.0.0.11:3306
trading-10.0.0.12:3306


This is a sensible improvement over identifying clients by service name alone.

However, I would improve the identity model.
A. Service identity and connection identity are different things
A service may restart while retaining the same address.

It may also have multiple instances behind the same advertised endpoint.

The current identity:
service name + endpoint


cannot distinguish every possible instance or connection lifecycle.

I would use:
type ClientIdentity struct {
    ServiceName string
    InstanceID  string
    Address     string
}


And maintain:
InstanceID → ClientMailbox


The service name becomes a grouping attribute rather than the unique key.

This also avoids accidental replacement when multiple instances advertise identical addresses.
B. Validate identity before registration
The handler checks:
if identity == nil {
    return
}


But the name and advertised address are extracted without checking their validity.

I would validate:
- Non-empty service name.
- Valid service name format.
- Valid advertised address when supplied.
- Instance identifier.
- Allowed service identity, where authentication is required.
An identity handshake is not necessarily authentication. A client that can connect may still be able to claim another service's identity unless the transport establishes trust separately.

This matters particularly because the service registry is distributed to other microservices.
C. The registry has nondeterministic ordering
There is a small but worthwhile improvement in broadcastRegistry().

The code builds slices by iterating over Go maps:
for name, mb := range *ptr {
    clients = append(clients, name)
    // ...
}


Go map iteration order is unspecified.

Consequently, two registry broadcasts containing exactly the same services can produce differently ordered arrays.

For example:
{
  "active_services": [
    "database",
    "trading",
    "analytics"
  ]
}


versus:
{
  "active_services": [
    "analytics",
    "database",
    "trading"
  ]
}


The information is identical, but the serialized output differs.

This can cause unnecessary client updates, confusing logs and unstable tests.

Simple correction: sort both active_services and service_addresses before serialization.

This is a small change with a good return on investment.

GitHub+1
8. Concurrent updates and broadcast ordering
There is another issue closely related to the previous ones.

Imagine two clients updating the configuration simultaneously:
Client A → PUT_SYNC (database.host = db-A)
Client B → PUT_SYNC (database.host = db-B)


The store's mutex ensures that updates are committed sequentially.

For example:
Revision 100
    |
    v
Client A commits → Revision 101
    |
    v
Client B commits → Revision 102


That part is good.

However, the broadcast system is separate from the store transaction.

BroadcastConfig() loads a snapshot and marshals it, then broadcasts it. Multiple callers can execute that operation concurrently.

A possible execution:
A commits revision 101
A starts broadcast, but gets delayed

B commits revision 102
B broadcasts revision 102

A resumes
A broadcasts revision 101


The clients could receive an older configuration after a newer one.

The atomic store protects the state itself, but does not automatically serialize notifications.
My recommendation: revision-aware broadcast
I would introduce a small synchronization mechanism around publication.

Conceptually:
Store.Update()
    |
    v
Commit revision N
    |
    v
Publish revision N
    |
    v
Mailbox


Every broadcast carries its revision.

Clients must never replace a newer revision with an older one.

The server should also coalesce obsolete pending updates.

I would avoid using a global mutex around network writes. The serialization should apply to publication decisions, not slow socket operations.

This gives you consistency without unnecessarily blocking configuration updates.

9. The mailbox should distinguish configuration updates from responses
The current implementation uses the same mailbox for:
- Broadcast configuration updates.
- Full refresh messages.
- Registry notifications.
- Individual request responses.
This is convenient, but it mixes two different types of traffic.

Control responses should not be dropped silently.

Configuration notifications can often be coalesced because a newer full snapshot supersedes an older one.

I would consider this model:
Message type	Queue policy
Request response	Guaranteed delivery or disconnect
Configuration update	Coalesce to latest revision
Full refresh	Highest priority
Registry update	Coalesce to latest registry
Shutdown	Explicit lifecycle signal

This would prevent a burst of registry notifications from consuming capacity needed for important request responses.

There is also a potential ordering ambiguity because responses and broadcasts are enqueued by different goroutines.

If the protocol requires a particular ordering between a response and its corresponding broadcast, that needs to be explicitly defined.

A simple way to address this is to include the committed configuration revision in the response.

GitHub+1
10. Proposed target architecture
I would retain the existing Go architecture, but simplify the ownership boundaries.
TCP / REST / gRPC / Telegram

Protocol adapters · validation · authorization


ConfigController

PATCH · REPLACE · DELETE · RELOAD


Versioned ConfigStore

Revision + immutable snapshot + atomic pointer swap

Persistence manager

Revision tracking · atomic save · retry


Distribution manager

Ordered publication · coalescing




Client mailboxes

Lifecycle-safe · independent queues · revision awareness


Microservice fleet

Consistent configuration snapshots


What I would retain
- Go and its concurrency primitives.
- atomic.Pointer for configuration snapshots.
- Deep Copy-On-Write.
- A single authoritative configuration store.
- Per-client mailboxes.
- Safe-Socket and protobuf.
- Atomic temporary-file persistence.
- Debounced persistence.
- Protocol-independent ConfigController.
- Full refresh functionality.
What I would change
- Add a revision to every configuration snapshot.
- Enforce snapshot ownership inside the store.
- Explicitly separate PATCH and REPLACE semantics.
- Prevent concurrent send/close mailbox races.
- Replace silent notification loss with revision-based recovery.
- Coalesce obsolete configuration broadcasts.
- Make persistence retry revision-aware.
- Propagate reload and filesystem errors.
- Sort registry output.
- Improve instance identity.
- Separate critical responses from coalescible notifications.

11. Testing strategy
I would add tests specifically targeting the concurrency model.
Test	Expected behavior
100 concurrent configuration updates	No lost updates
Concurrent readers during updates	Readers see complete snapshots
Failed modification callback	Original snapshot remains unchanged
Callback retains returned map	Published state cannot be modified externally
Two concurrent broadcasts	Older revision cannot overwrite newer revision
Disconnect during broadcast	No panic
1,000 clients registering simultaneously	No lost registrations
Two instances with same service name	Both remain registered
Full mailbox	Recovery through latest snapshot
Persistence failure	Automatic retry
Reload corrupted JSON	Error, previous state preserved
Server shutdown during save	No accepted update silently lost
Registry rebuild	Deterministic serialization

One test I particularly recommend is a randomized concurrency test under Go's race detector:
go test -race ./...


Generate hundreds of concurrent updates, registrations, disconnects and broadcasts.

The important thing is to test not only whether the application crashes, but whether all clients eventually converge to the same configuration revision.

12. Implementation priorities
Phase 1 — Critical correctness
Priority 1

- [ ] Fix mailbox send/close race.
- [ ] Fix persistence retry after failure.
- [ ] Make reload errors explicit.
- [ ] Introduce configuration revisions.
- [ ] Prevent stale broadcasts from replacing newer snapshots.

Phase 2 — Configuration semantics
Priority 2

- [ ] Formalize PATCH / REPLACE / DELETE.
- [ ] Enforce immutable snapshot ownership.
- [ ] Ensure full refresh handles deleted keys.
- [ ] Define merge behavior for missing sections and empty values.

Phase 3 — Distribution reliability
Priority 3

- [ ] Coalesce configuration notifications.
- [ ] Implement client revision gap detection.
- [ ] Separate response and broadcast policies.
- [ ] Improve instance identity.
- [ ] Make registry ordering deterministic.

Phase 4 — Performance
Priority 4

- [ ] Benchmark COW costs under write-heavy workloads.
- [ ] Measure broadcast throughput at 100, 500 and 1,000 clients.
- [ ] Measure JSON serialization overhead.
- [ ] Benchmark mailbox capacities and coalescing.
- [ ] Evaluate worker pools only if profiling demonstrates a bottleneck.

13. Overall assessment
8 / 10
Architectural potential

Qualitative assessment of the inspected implementation, not a measured performance score.
Area	Assessment
Architecture	Very good
Copy-On-Write store	Very good
Atomic snapshots	Very good
Configuration merging	Good, needs formal semantics
Persistence	Good, retry issue
Client lifecycle	Needs correction
Broadcast consistency	Needs correction
Scalability	Promising
Maintainability	Good potential


The developer has made some interesting architectural choices here.

In particular, the immutable snapshot approach is appropriate for a configuration server because configuration reads vastly outnumber writes in most microservice environments.

I would not replace that with ordinary mutex-protected reads.

The main weaknesses are around the boundaries between otherwise well-designed components:
- Store versus broadcaster.
- Broadcaster versus mailbox lifecycle.
- In-memory state versus persistence.
- Partial update versus full replacement.
These boundaries need clearer guarantees.

14. One thing I would investigate beyond this repository
There is an important relationship between config-server and distributed-config.

The server owns the authoritative live configuration, but the client library also has its own local configuration state, merge rules, profile strategies and atomic pointer swaps.

That means there are effectively two configuration engines:
1. The server-side configuration engine.
2. The client-side configuration engine.
They must agree on precedence, deletion, replacement, revisions and synchronization.

Otherwise, the server can be perfectly correct while individual microservices still end up with different configurations.

I would therefore make a separate cross-repository review of these two components before considering the configuration infrastructure finished.

Final recommendation
My preferred approach would be to make three foundational changes first:
1. Versioned immutable configuration snapshots.
2. A lifecycle-safe mailbox and revision-aware broadcast system.
3. Revision-aware persistence with automatic retries.
These three changes solve a large proportion of the reliability concerns without requiring a major architectural rewrite.
Sources inspected
- Store — store.go
- Persistence — persistence.go
- Server — server.go
- Connection lifecycle — connection.go
- Architecture
- TODO
The review concerns the inspected develop branch. I have not executed the Go test suite or race detector, so concurrency findings are code-level findings rather than reproduced runtime failures.

My suggested next step: review distributed-config together with config-server, specifically the merge and precedence rules on both sides. That is where I suspect we can uncover the most important remaining architectural inconsistencies in the configuration system.