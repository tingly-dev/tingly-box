package serverguardrails

// blockMessageForEvaluation builds the user-visible block text from a completed
// evaluation. Command responses get a command-specific message; plain text
// responses use a preview snippet.
func blockMessageForEvaluation(evaluation Evaluation) string {
	if evaluation.Input.Content.Command != nil {
		return BlockMessageForCommand(
			evaluation.Result,
			evaluation.Input.Content.Command.Name,
			evaluation.Input.Content.Command.Arguments,
		)
	}
	return BlockMessageWithSnippet(evaluation.Result, evaluation.Input.Content.Preview(120))
}
