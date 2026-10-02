package whatsmeow_service

import "sync"

// ClientMapsMu guards the three instance-keyed maps created once in
// cmd/evolution-go/main.go — killChannel (map[string]chan bool), clientPointer
// (map[string]*whatsmeow.Client) and myClientPointer (map[string]*MyClient) —
// and shared by reference across every service package in this module
// (whatsmeow, instance, sendMessage, chat, call, community, group, label,
// message, newsletter, user) as well as every *MyClient value, which keeps its
// own copies of the same three map references.
//
// Because all of those packages read and write the very same underlying maps
// concurrently (HTTP handlers reading a client to act on it while
// ReconnectClient/StartClient add or remove entries from a background
// goroutine), any indexed access — assignment, delete, membership check,
// range, or len — MUST hold this mutex. Every read (`v := m[k]`, `v, ok :=
// m[k]`, `range m`, `len(m)`) takes RLock/RUnlock; every write (`m[k] = v`,
// `delete(m, k)`) takes Lock/Unlock. Without this, Go's runtime fatals the
// whole process on a concurrent map read/write or concurrent map writes — it
// is not a panic that can be recovered from.
//
// Rule of thumb: never hold the lock across a long or blocking call —
// Disconnect/Connect/Logout, StartInstance/ReconnectClient/StartClient, a send
// on a kill channel, or any network I/O. Copy the map value you need into a
// local variable while holding the lock, release the lock, then operate on
// the local copy. This also avoids deadlocks: several of the guarded
// functions call into each other (ReconnectClient -> StartInstance ->
// StartClient), and none of them may still be holding ClientMapsMu when they
// do.
var ClientMapsMu sync.RWMutex
