package state

import "time"

type ContextEvent struct {
	Action       string
	Path         string
	ReadOnly     bool
	External     bool
	Version      string
	PrevVersion  string
	OriginalPath string // For UserSaveAs: the path LLM suggested
	Added        bool   // For AssistantWriteFile: true if new file, false if modified
	StartLine    int    // For sections: 1-indexed start line
	EndLine      int    // For sections: 1-indexed end line (inclusive)
}

func (s *State) addContextEvent(event ContextEvent) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	readOnly := event.ReadOnly
	external := event.External

	msg := Message{
		Role:      "assistant",
		Timestamp: time.Now().UTC(),
		Parts: []MessagePart{{
			Type:         "context_event",
			Action:       event.Action,
			Path:         event.Path,
			ReadOnly:     &readOnly,
			External:     &external,
			Version:      event.Version,
			PrevVersion:  event.PrevVersion,
			OriginalPath: event.OriginalPath,
			Added:        event.Added,
			StartLine:    event.StartLine,
			EndLine:      event.EndLine,
		}},
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

	return s.addContextEvent(ContextEvent{
		Action:   "AssistantWriteFile",
		Path:     path,
		ReadOnly: readOnly,
		External: external,
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

	return s.addContextEvent(ContextEvent{
		Action:      "UserRejectOutput",
		Path:        path,
		ReadOnly:    readOnly,
		External:    external,
		PrevVersion: prevVersion,
	})
}
