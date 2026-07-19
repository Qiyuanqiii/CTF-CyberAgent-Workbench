package application

import (
	"strconv"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/contextmgr"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/redact"
)

const modelMessageFramingTokens = 4

type modelContextLayout struct {
	HistoryStart int
	HistoryCount int
}

func (l modelContextLayout) shifted(offset int) modelContextLayout {
	l.HistoryStart += offset
	return l
}

type modelContextPlan struct {
	WindowTokens       int
	InputLimitTokens   int
	EstimatedInput     int
	OutputLimitTokens  int
	HistoryOmitted     int
	SafetyMarginTokens int
}

func constrainRequestToModelWindow(request llm.ChatRequest, window llm.ContextWindow,
	layout modelContextLayout,
) (llm.ChatRequest, modelContextPlan, error) {
	if err := window.Validate(); err != nil {
		return llm.ChatRequest{}, modelContextPlan{}, apperror.Wrap(
			apperror.CodeFailedPrecondition, "model context-window policy is invalid", err)
	}
	if layout.HistoryStart < 0 || layout.HistoryCount < 0 ||
		layout.HistoryStart > len(request.Messages) ||
		layout.HistoryCount > len(request.Messages)-layout.HistoryStart {
		return llm.ChatRequest{}, modelContextPlan{}, apperror.New(
			apperror.CodeFailedPrecondition, "model context history layout is invalid")
	}
	request.Messages = append([]llm.Message(nil), request.Messages...)
	request.Tools = append([]llm.ToolSpec(nil), request.Tools...)
	request.MaxTokens = window.OutputLimit(request.MaxTokens)
	inputLimit, err := window.InputLimit(request.MaxTokens)
	if err != nil {
		return llm.ChatRequest{}, modelContextPlan{}, apperror.Wrap(
			apperror.CodeFailedPrecondition, "model context input limit is invalid", err)
	}
	omitted := 0
	estimated := estimateModelRequestTokens(request)
	for estimated > inputLimit && layout.HistoryCount > 0 {
		request.Messages = append(request.Messages[:layout.HistoryStart],
			request.Messages[layout.HistoryStart+1:]...)
		layout.HistoryCount--
		omitted++
		estimated = estimateModelRequestTokens(request)
	}
	if estimated > inputLimit {
		return llm.ChatRequest{}, modelContextPlan{}, apperror.New(
			apperror.CodeResourceExhausted,
			"mandatory model context exceeds the conservative input window")
	}
	plan := modelContextPlan{
		WindowTokens: window.WindowTokens, InputLimitTokens: inputLimit,
		EstimatedInput: estimated, OutputLimitTokens: request.MaxTokens,
		HistoryOmitted: omitted, SafetyMarginTokens: window.SafetyMarginTokens,
	}
	metadata := make(map[string]string, len(request.Metadata)+8)
	for key, value := range request.Metadata {
		metadata[key] = value
	}
	metadata["context_protocol"] = window.ProtocolVersion
	metadata["context_window_source"] = redact.String(window.Source)
	metadata["context_window_tokens"] = strconv.Itoa(plan.WindowTokens)
	metadata["context_input_limit"] = strconv.Itoa(plan.InputLimitTokens)
	metadata["context_input_estimate"] = strconv.Itoa(plan.EstimatedInput)
	metadata["context_output_limit"] = strconv.Itoa(plan.OutputLimitTokens)
	metadata["context_safety_margin"] = strconv.Itoa(plan.SafetyMarginTokens)
	metadata["context_history_omitted"] = strconv.Itoa(plan.HistoryOmitted)
	request.Metadata = metadata
	return request, plan, nil
}

func estimateModelRequestTokens(request llm.ChatRequest) int {
	total := 8
	for _, message := range request.Messages {
		total = addModelTokens(total, modelMessageFramingTokens)
		total = addModelTokens(total, contextmgr.EstimateTokens(message.Role))
		total = addModelTokens(total, contextmgr.EstimateTokens(message.Content))
		for _, call := range message.ToolCalls {
			total = addModelTokens(total, contextmgr.EstimateTokens(call.ID))
			total = addModelTokens(total, contextmgr.EstimateTokens(call.Name))
			total = addModelTokens(total, contextmgr.EstimateTokens(string(call.Arguments)))
		}
		for _, result := range message.ToolResults {
			total = addModelTokens(total, contextmgr.EstimateTokens(result.ToolCallID))
			total = addModelTokens(total, contextmgr.EstimateTokens(result.Content))
		}
	}
	for _, tool := range request.Tools {
		total = addModelTokens(total, modelMessageFramingTokens)
		total = addModelTokens(total, contextmgr.EstimateTokens(tool.Name))
		total = addModelTokens(total, contextmgr.EstimateTokens(tool.Description))
		total = addModelTokens(total, contextmgr.EstimateTokens(string(tool.Parameters)))
	}
	if request.JSONMode {
		total = addModelTokens(total, 8)
	}
	return total
}

func addModelTokens(current int, addition int) int {
	if addition <= 0 {
		return current
	}
	maxInt := int(^uint(0) >> 1)
	if current > maxInt-addition {
		return maxInt
	}
	return current + addition
}
