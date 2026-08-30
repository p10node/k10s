package ui

// The AI prompt is switched off for now.
//
// It is being reworked, and until that lands it is not something to leave
// reachable: the assistant answers with a live cluster's context attached,
// against whatever endpoint and key happen to be configured, and a
// half-finished version of that is a bad thing to hand somebody by accident.
//
// So rather than deleting the code, every way in — ctrl+a, the [ CMD ]/[ AI ]
// badge, the settings fields — routes here and says so. Flipping this back
// to false is all it takes to restore the feature.
const aiDisabled = true

// noticeAI is what any attempt to reach the AI prompt gets: a modal saying
// the feature is off, rather than a toast that scrolls away unread.
func (m *Model) noticeAI() {
	m.confirm = &confirmState{
		title:  "AI is disabled",
		notice: true,
		message: []string{
			"The AI prompt is turned off in this build.",
			"",
			"It is being reworked, and it stays off until that",
			"is done — an assistant wired to a live cluster is",
			"not something to ship half-finished.",
			"",
			"Everything else works as before: type a command,",
			"or use \":\" for resources and \"/\" for k10s itself.",
		},
	}
}
