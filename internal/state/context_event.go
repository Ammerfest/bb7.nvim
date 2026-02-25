package state

import "time"

// addContextEvent appends a context_event message to the active chat.
func (s *State) addContextEvent(part MessagePart) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	msg := Message{
		Role:      "assistant",
		Timestamp: time.Now().UTC(),
		Parts:     []MessagePart{part},
	}

	s.ActiveChat.Messages = append(s.ActiveChat.Messages, msg)
	return s.SaveActiveChat()
}

// AssistantWriteFile records that the assistant wrote a file version to output.
// isNew indicates whether this is a new file (true) or modification of existing context file (false).
func (s *State) AssistantWriteFile(path, content string, isNew bool) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	cf := s.findContextFile(path)
	readOnly := false
	external := false
	if cf != nil {
		readOnly = cf.ReadOnly
		external = cf.External
	}

	return s.addContextEvent(MessagePart{
		Type:     PartTypeContextEvent,
		Action:   ActionAssistantWriteFile,
		Path:     path,
		ReadOnly: &readOnly,
		External: &external,
		Version:  HashFileVersion(path, content),
		Added:    isNew,
	})
}

// UserRejectOutput records that the user rejected/deleted an output file.
func (s *State) UserRejectOutput(path string) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	cf := s.findContextFile(path)
	readOnly := false
	external := false
	var prevVersion string
	if cf != nil {
		readOnly = cf.ReadOnly
		external = cf.External
		prevVersion = cf.Version
	}

	return s.addContextEvent(MessagePart{
		Type:        PartTypeContextEvent,
		Action:      ActionUserRejectOutput,
		Path:        path,
		ReadOnly:    &readOnly,
		External:    &external,
		PrevVersion: prevVersion,
	})
}
