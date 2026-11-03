package whatsmeow_service

func isConnectionEvent(name string) bool {
	switch name {
	case "Connected", "PairSuccess", "TemporaryBan", "LoggedOut", "ConnectFailure", "Disconnected", "ClientOutdated", "StreamReplaced":
		return true
	}
	return false
}
