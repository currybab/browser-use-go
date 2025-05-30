package agent

import (
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/nerdface-ai/browser-use-go/internals/controller"
	"github.com/nerdface-ai/browser-use-go/pkg/browser"

	"github.com/cloudwego/eino/schema"
)

//go:embed system_prompt.md
var template embed.FS

type SystemPrompt struct {
	SystemMessage            *schema.Message
	DefaultActionDescription string
	MaxActionsPerStep        int
}

func NewSystemPrompt(
	actionDescription string,
	maxActionsPerStep int,
	overrideSystemMessage *string,
	extendSystemMessage *string,
) *SystemPrompt {
	sp := &SystemPrompt{
		DefaultActionDescription: actionDescription,
		MaxActionsPerStep:        maxActionsPerStep,
	}
	var prompt string
	if overrideSystemMessage != nil {
		prompt = *overrideSystemMessage
	} else {
		loaded := sp.loadPromptTemplate()
		prompt = strings.Replace(loaded, "{max_actions}", fmt.Sprintf("%d", sp.MaxActionsPerStep), -1)
	}

	if extendSystemMessage != nil {
		prompt += fmt.Sprintf("\n%s", *extendSystemMessage)
	}

	sp.SystemMessage = &schema.Message{
		Role:    schema.System,
		Content: prompt,
	}
	return sp
}

func (sp *SystemPrompt) loadPromptTemplate() string {
	// Load the prompt template from the markdown file
	data, err := template.ReadFile("system_prompt.md")
	if err != nil {
		panic(err)
	}
	return string(data)
}

type AgentMessagePrompt struct {
	State             *browser.BrowserState
	Result            []*controller.ActionResult
	IncludeAttributes []string
	StepInfo          *AgentStepInfo
}

func NewAgentMessagePrompt(
	state *browser.BrowserState,
	result []*controller.ActionResult,
	includeAttributes []string,
	stepInfo *AgentStepInfo,
) *AgentMessagePrompt {
	return &AgentMessagePrompt{
		State:             state,
		Result:            result,
		IncludeAttributes: includeAttributes,
		StepInfo:          stepInfo,
	}
}

func (amp *AgentMessagePrompt) GetUserMessage(useVision bool) *schema.Message {
	// get specific attribute clickable elements in DomTree as string
	elementText := amp.State.ElementTree.ClickableElementsToString(amp.IncludeAttributes)

	hasContentAbove := amp.State.PixelAbove > 0
	hasContentBelow := amp.State.PixelBelow > 0

	if elementText != "" {
		if hasContentAbove {
			elementText = fmt.Sprintf("... %d pixels above - scroll or extract content to see more ...\n%s", amp.State.PixelAbove, elementText)
		} else {
			elementText = fmt.Sprintf("[Start of page]\n%s", elementText)
		}
		// Update elementText by appending the new info to the existing value
		if hasContentBelow {
			elementText = fmt.Sprintf("%s\n... %d pixels below - scroll or extract content to see more ...", elementText, amp.State.PixelBelow)
		} else {
			elementText = fmt.Sprintf("%s\n[End of page]", elementText)
		}
	} else {
		elementText = "empty page"
	}

	var stepInfoDescription string
	if amp.StepInfo != nil {
		current := int(amp.StepInfo.StepNumber) + 1
		max := int(amp.StepInfo.MaxSteps)
		stepInfoDescription = fmt.Sprintf("Current step: %d/%d", current, max)
	} else {
		stepInfoDescription = ""
	}
	timeStr := time.Now().Format("2006-01-02 15:04")
	stepInfoDescription += fmt.Sprintf("Current date and time: %s", timeStr)

	stateDescription := fmt.Sprintf(`
[Task history memory ends]
[Current state starts here]
The following is one-time information - if you need to remember it write it to memory:
Current url: %s
Available tabs:
%s
Interactive elements from top layer of the current page inside the viewport:
%s
%s`,
		amp.State.Url,
		browser.TabsToString(amp.State.Tabs),
		elementText,
		stepInfoDescription,
	)

	if amp.Result != nil {
		for i, result := range amp.Result {
			if result.ExtractedContent != nil {
				stateDescription += fmt.Sprintf("\nAction result %d/%d: %s", i+1, len(amp.Result), *result.ExtractedContent)
			}
			if result.Error != nil {
				// only use last line of error
				errStr := *result.Error
				splitted := strings.Split(errStr, "\n")
				lastLine := splitted[len(splitted)-1]
				stateDescription += fmt.Sprintf("\nAction error %d/%d: ...%s", i+1, len(amp.Result), lastLine)
			}
		}
	}

	if amp.State.Screenshot != nil && useVision {
		// Format message for vision model
		return &schema.Message{
			Role: schema.User,
			MultiContent: []schema.ChatMessagePart{
				{
					Type: schema.ChatMessagePartTypeText,
					Text: stateDescription,
				},
				{
					Type: schema.ChatMessagePartTypeImageURL,
					ImageURL: &schema.ChatMessageImageURL{
						URL: "data:image/png;base64," + *amp.State.Screenshot,
					},
				},
			},
		}
	}

	return &schema.Message{
		Role:    schema.User,
		Content: stateDescription,
	}
}
