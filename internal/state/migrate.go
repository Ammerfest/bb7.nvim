package state

// CurrentChatVersion is the latest chat format version.
// Version 0/1: legacy format with Message.Content field.
// Version 2: Parts-only format (Content migrated into Parts).
const CurrentChatVersion = 2

// migrateChat runs all applicable migrations on a chat.
// Returns true if any migration was applied.
//
// The v0/v1 → v2 content-to-parts migration is handled transparently by
// Message.UnmarshalJSON, so by the time this function runs the struct is
// already in the v2 shape. This function bumps the version number so the
// chat is re-saved in the new format.
func migrateChat(chat *Chat) bool {
	if chat.Version >= CurrentChatVersion {
		return false
	}
	chat.Version = CurrentChatVersion
	return true
}
