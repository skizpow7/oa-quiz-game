# Overachiever CLI Quiz Game (Go)

A complicated **command-line quiz game** written in Go and BubbleTea that generates questions randomly based on a selected 
skill level and quizzes the user interactively via the terminal.

## Original Challenge

Create a simple command line quiz that has a predetermined set of easy math questions and answers. Program should keep track of
time and stop user after a set amount of time. Program should also be able to determine correct / incorrect answers and show
totals at the end of the quiz.

## Above and Beyond

### Features added

BubbleTea Go package - allows for multiple built-ins: colors, menus, timers, and app state.  
Difficulty selection.  
Time limit selection.  
Random math question generation based on difficulty selection.  
Keeps track of time elapsed per question.  
Keeps track of time elapsed per question type.  
Keeps track of time elapsed per math branch.  
Keeps track of incorrect answers and the correct solution.  
Timer visually shown.  

### Known Issues

Timer bar colors and pulses do not reset between game iterations.  
Console overflow - if a user answers too many questions wrong that all solutions do not fit in the console window, program 
does not adjust for scrolling.

## Explanation of Code

### Setup

```go
type appState int

const (
	stateMenu appState = iota
	stateTimeSelect
	stateRunning
	stateResults

	clearScreen = "\033[H\033[2J"
)
```  
This appState variable keeps track of where the app is in terms of which part of the quiz is active.
The options are stateMenu (main menu), stateTimeSelect (time select screen), stateRunning (quiz active), and
stateResults (final results screen).  
clearScreen is a constant that's called to help mitigate the console overflow issue.    

```go
type difficultyItem struct {
	name string
	desc string
}

func (d difficultyItem) Title() string       { return d.name }
func (d difficultyItem) Description() string { return d.desc }
func (d difficultyItem) FilterValue() string { return d.name }
```  
Part of the BubbleTea package. These interfaces are for the built-in menu feature of Bubble Tea.  

```go
type Question struct {
	Text     string
	Answer   int
	OpType   string
	UniqueID string
}

type AnswerRecord struct {
	Question  Question
	Correct   bool
	Duration  time.Duration
	UserInput string
}
```  
The two major structs of the quiz.    
Question keeps the text version of the question, the answer as an integer (all answers are integer),
what type of math question it is, and a unique id to ensure questions aren't repeated.    
AnswerRecord keeps the related Question, Boolean correct, how long it took to answer, and the user's input.  

```go
type model struct {
	state      appState
	difficulty string
	timeLimit  int

	list      list.Model
	timeList  list.Model
	textInput textinput.Model

	currentQ      Question
	usedQuestions map[string]struct{}
	answers       []AnswerRecord
	startTime     time.Time
	totalStart    time.Time
	questionStart time.Time
	timeRemaining time.Duration
	timerActive   bool
	suspendTick   bool

	flashText          string
	flashColor         string // "green" or "red"
	flashActive        bool
	flashBarAdjust     int
	flashFadeSteps     int
	flashColorOverride string
	fuseFrameIndex     int
	fuseFrames         []string
}
```

The app's full model:  
* state - what mode the app is in
* difficulty - selected difficulty
* timeLimit - selected time limit  
* list - difficulty list
* timeList - time limit list
* textInput - BubbleTea model for user input
* currentQ - the currently shown question
* usedQuestions - a map to keep track of unique questions
* answers - slice of AnswerRecords
* startTime - set when quiz begins
* totalStart - total elapsed time of quiz
* questionStart - elapsed time of current question
* timeRemaining - time left in quiz (variable, can change outside timeLimit input)
* timerActive - Boolean to keep track of timer state
* suspendTick - used to determine if user input is suspended
* flash*... - variables used to change visual elements

```go
func initialModel() model {
	diffItems := []list.Item{
		difficultyItem{"1st Grade", "Basic single-digit addition and subtraction"},
		difficultyItem{"3rd Grade", "Larger numbers and simple multiplication"},
		difficultyItem{"5th Grade", "Two-digit operations"},
		difficultyItem{"Algebra", "Variables and expressions"},
	}

	timeItems := []list.Item{
		difficultyItem{"30s", "Brain Storm"},
		difficultyItem{"60s", "Normal Person"},
		difficultyItem{"90s", "Ok, Boomer"},
		difficultyItem{"2m", "Marathon"},
	}

	const defaultWidth = 80
	const defaultHeight = 20
	diffList := list.New(diffItems, list.NewDefaultDelegate(), defaultWidth, defaultHeight)
	diffList.Title = "Select Difficulty Level"

	timeList := list.New(timeItems, list.NewDefaultDelegate(), defaultWidth, defaultHeight)
	timeList.Title = "Select Quiz Duration"

	textInput := textinput.New()
	textInput.Placeholder = "Your answer"
	textInput.Focus()
	textInput.CharLimit = 5
	textInput.Width = 20

	fuseFrames := []string{"*", "✨", "·", "✶"}

	return model{
		state:          stateMenu,
		list:           diffList,
		timeList:       timeList,
		textInput:      textInput,
		usedQuestions:  make(map[string]struct{}),
		fuseFrames:     fuseFrames,
		fuseFrameIndex: 0,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fuseTick(), pulseTick())
}
```  

Sets initial model:
* diffItems - list of difficulties
* timeItems - list of time limits
* defaultWidth - correction for console overflow issue
* defaultHeight - correction for console overflow issue

The rest of this block sets the various states and variables in the model.

#### Update

```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateMenu:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)

		switch msg := msg.(type) {

		case tea.KeyMsg:
			if msg.Type == tea.KeyEnter {
				if selected, ok := m.list.SelectedItem().(difficultyItem); ok {
					m.difficulty = selected.name
					m.state = stateTimeSelect
				}
			} else if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
				return m, tea.Quit
			}
		}
		return m, cmd

	case stateTimeSelect:
		var cmd tea.Cmd
		m.timeList, cmd = m.timeList.Update(msg)

		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {

			case "enter":
				if selected, ok := m.timeList.SelectedItem().(difficultyItem); ok {
					switch selected.name {
					case "30s":
						m.timeLimit = 30
					case "60s":
						m.timeLimit = 60
					case "90s":
						m.timeLimit = 90
					case "2m":
						m.timeLimit = 120
					}
					m.currentQ = generateQuestion(m.difficulty, m.usedQuestions)
					m.totalStart = time.Now()
					m.questionStart = time.Now()
					m.timeRemaining = time.Duration(m.timeLimit) * time.Second
					m.state = stateRunning
					m.startTime = time.Now()
					return m, tea.Batch(tick(), fuseTick(), pulseTick())
				}

			case "left":
				m.state = stateMenu

			case "q":
				return m, tea.Quit
			}
		}
		return m, cmd
```  

The model updater method.  
First, the function determines what state the model is in. If in stateMenu or 
stateTimeSelect, those selections are shown through the BubbleTea built-in menu system. It can also handle
quit and back functions. Once a time limit is selected, the quiz starts and questions and timers are set.   

```go
case stateRunning:
		var cmds []tea.Cmd
		var cmd tea.Cmd

		// Timer countdown tick
		switch msg.(type) {
		case tickMsg:
			if !m.suspendTick {
				m.timeRemaining -= time.Second
			}

			if m.timeRemaining <= 0 {
				fmt.Print("\a") // beep
				m.state = stateResults
				return m, nil
			}

			cmds = append(cmds, tick())
		case fuseTickMsg:
			m.fuseFrameIndex = (m.fuseFrameIndex + 1) % len(m.fuseFrames)
			return m, fuseTick()
		case pulseTickMsg:
			if m.timeRemaining <= 10*time.Second {
				m.flashFadeSteps = 1
				if m.flashColorOverride == "" {
					m.flashColorOverride = "brightRed"
				} else {
					m.flashColorOverride = ""
				}
			}
			return m, pulseTick()
		}

		// Handle text input
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)

		switch msg := msg.(type) {
		case flashDoneMsg:
			m.flashActive = false
			m.flashColorOverride = ""
			m.flashBarAdjust = 0
			m.textInput.Reset()
			m.textInput.Focus()
			m.currentQ = generateQuestion(m.difficulty, m.usedQuestions)
			m.suspendTick = false
			m.questionStart = time.Now()
		case flashFadeMsg:
			if m.flashFadeSteps > 0 {
				m.flashFadeSteps--
				if m.flashFadeSteps == 0 {
					m.flashColorOverride = ""
				} else {
					return m, flashFadeTick()
				}
			}
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				userInput := m.textInput.Value()
				var userAns int
				_, err := fmt.Sscanf(userInput, "%d", &userAns)
				if err == nil {
					correct := userAns == m.currentQ.Answer
					m.answers = append(m.answers, AnswerRecord{
						Question:  m.currentQ,
						Correct:   correct,
						Duration:  time.Since(m.questionStart),
						UserInput: userInput,
					})
					if correct {
						m.flashText = "Correct!"
						m.flashColor = "green"
						m.flashBarAdjust = 1
						m.flashColorOverride = "brightGreen"
						m.timeRemaining += time.Second
						if m.timeRemaining > time.Duration(m.timeLimit)*time.Second {
							m.timeRemaining = time.Duration(m.timeLimit) * time.Second
						}
					} else {
						m.flashText = "Incorrect!"
						m.flashColor = "red"
						m.flashBarAdjust = -1
						m.flashColorOverride = "brightRed"
						m.timeRemaining -= time.Second
						if m.timeRemaining < time.Second {
							m.timeRemaining = time.Second
						}
					}

					m.flashFadeSteps = 3
					m.suspendTick = true
					m.flashActive = true

					// Don't generate next question yet — wait 1 sec
					m.textInput.Blur()
					return m, tea.Batch(flashTimeout(), flashFadeTick())
				}
			case "q":
				m.state = stateMenu
				m.list.Select(0)
				m.timeList.Select(0)
				m.textInput.Reset()
				m.usedQuestions = make(map[string]struct{})
				m.answers = nil
				return m, nil

			}
		}
		return m, tea.Batch(cmds...)
```  

If the quiz is running, it enters the above block. BubbleTea runs on commands and messages. The first part of this block sets up a
slice of commands and an empty command. Then it checks its messages.  
On a tickMsg, if the suspendTick has not been activated,
it removes a second from the timeRemaining timer. If the main timer is empty, it ends the quiz and changes the state to stateResults. Otherwise, it performs its tick() command.  
If the message is a fuseTickMsg, it advances the fuseFrame visual.   
If it's a pulseTickMsg, it determines the correct state of the color pulse.  
  
It then checks to see if there was input. Before deciding what to do on the input, it checks to see if the correct/incorrect message is finished.
If so, it hides the flash, sets the timer bar back to normal (discussed later), refocuses to the input, generates a new question, and starts both the main timer and a new 
question timer. If the flash is not done, it advances its flash timer by one.  
  
If the message was a keystroke entry, if the key entered was "q", it exits the quiz. Otherwise, it checks the input answer.
It takes the string value, converts it to an integer (if possible), and records both the question and user answer for display later.
If the answer is correct, it stops and adds one second to the main timer and shows the "correct" flash. If incorrect,
it stops and subtracts a second from the main timer and shows the "incorrect" flash. The user's time limit is therefore not influenced by the flash message, only
bonus/penalty for the correct/incorrect answer.  

```go
case stateResults:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if msg.String() == "left" {
				m.state = stateMenu
				m.list.Select(0)
				m.timeList.Select(0)
				m.textInput.Reset()
				m.answers = nil
				m.usedQuestions = make(map[string]struct{})
			}
		}
	default:
		m.state = stateMenu
	}
```  

If the quiz is over, the function does some cleanup and gathers all the results to pass to the View function.
If the appState is corrupted, the default is to return to the main menu.  

#### View

```go
func (m model) View() string {
	switch m.state {
	case stateMenu:
		return clearScreen + m.list.View() + "\nFullscreen recommended"

	case stateTimeSelect:
		return clearScreen + m.timeList.View() + "\nCorrect answers add 1s to time, incorrect subtracts 1s.\nTry to keep the bomb from exploding!\n[←] to go back | [q] to quit"

	case stateRunning:
		correct := 0
		for _, a := range m.answers {
			if a.Correct {
				correct++
			}
		}

		return clearScreen + lipgloss.NewStyle().
			Padding(1, 2).
			Render(fmt.Sprintf(
				"Time Left: %ds\n%s\nCorrect: %d/%d\n\n%s\n\n%s\n[q] to quit\n%s",
				int(m.timeRemaining.Seconds()),
				renderCountdownBar(int(m.timeRemaining.Seconds()), m.timeLimit, 30, m.flashBarAdjust, m.flashColorOverride, m.fuseFrames[m.fuseFrameIndex]),
				correct, len(m.answers),
				m.currentQ.Text,
				m.textInput.View(),
				func() string {
					if m.flashActive {
						style := lipgloss.NewStyle().
							Foreground(lipgloss.Color("10")) // green
						if m.flashColor == "red" {
							style = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
						}
						return "\n\n" + style.Bold(true).Render(m.flashText)
					}
					return ""
				}(),
			))

```  
The views are relatively simple. On appState stateMenu or stateTimeSelect, those menus are shown. If the appState is stateRunning,
the timer, question, and input areas are shown.  

```go
case stateResults:
		var correct, total int
		var totalDuration time.Duration
		opDurations := make(map[string][]time.Duration)

		// Gather stats and missed Qs
		missed := []string{}
		for i, a := range m.answers {
			total++
			if a.Correct {
				correct++
			} else {
				missed = append(missed, fmt.Sprintf(
					"  Question #%d: %s\n    Your Answer: %s\n    Correct Answer: %d",
					i+1, a.Question.Text, a.UserInput, a.Question.Answer))
			}
			totalDuration += a.Duration
			opDurations[a.Question.OpType] = append(opDurations[a.Question.OpType], a.Duration)
		}

		result := fmt.Sprintf("Final Score: %d / %d\n", correct, total)

		if total > 0 {
			avgTotal := totalDuration / time.Duration(total)
			result += fmt.Sprintf("Avg time (overall): %.2fs\n", avgTotal.Seconds())
		}

		// Group op types
		groups := map[string][]string{
			"Arithmetic": {},
			"Algebra":    {},
		}

		for op := range opDurations {
			if strings.HasPrefix(op, "algebra_") {
				groups["Algebra"] = append(groups["Algebra"], op)
			} else {
				groups["Arithmetic"] = append(groups["Arithmetic"], op)
			}
		}

		// Print groups in order
		for _, group := range []string{"Arithmetic", "Algebra"} {
			if len(groups[group]) > 0 {
				result += fmt.Sprintf("\n%s:\n", group)
				for _, op := range groups[group] {
					times := opDurations[op]
					var total time.Duration
					for _, t := range times {
						total += t
					}
					avg := total / time.Duration(len(times))
					label := strings.TrimPrefix(op, "algebra_")
					if group == "Algebra" {
						label = "algebra (" + label + ")"
					}
					result += fmt.Sprintf("  Avg time (%s): %.2fs\n", label, avg.Seconds())
				}
			}
		}

		if len(missed) > 0 {
			result += "\nIncorrect Answers:\n"
			for _, m := range missed {
				result += m + "\n"
			}
		}

		result += "\n[←] to play again"

		if m.timeRemaining <= 0 {
			result = "\n\n💣💥 BOOM! TIME’S UP! 💥💣\n\n" + result
		}

		return clearScreen + lipgloss.NewStyle().Padding(1, 2).Render(result)

```  

The majority of the appState stateResults is done here. It determines any missed questions and 
formats them into a readable string. It also calculates the user's overall average time, time per type of math question (arithmetic or algebra), 
time per subtype (addition, subtraction, etc.) and groups those into different text builders. It also then displays any missed questions along with 
the user's answer and the correct answer.

## Helpers

### Question Generator

```go
func generateQuestion(difficulty string, used map[string]struct{}) Question {
	var q Question

	for {
		var a, b, answer int
		var text, op string
		var opType string

		switch difficulty {
		case "1st Grade":
			a, b = rand.Intn(11), rand.Intn(11)
			if rand.Intn(2) == 0 {
				op = "+"
				answer = a + b
			} else {
				if a < b {
					a, b = b, a // prevent negative
				}
				op = "-"
				answer = a - b
			}
			text = fmt.Sprintf("%d %s %d = ?", a, op, b)
			opType = map[string]string{"+": "addition", "-": "subtraction"}[op]

		case "3rd Grade":
			a, b = rand.Intn(21), rand.Intn(21)
			ops := []string{"+", "-", "*"}
			op = ops[rand.Intn(len(ops))]
			switch op {
			case "+":
				answer = a + b
			case "-":
				answer = a - b
			case "*":
				// Single-digit multiplication only
				a, b = rand.Intn(10), rand.Intn(10)
				answer = a * b
			}
			text = fmt.Sprintf("%d %s %d = ?", a, op, b)
			opType = map[string]string{"+": "addition", "-": "subtraction", "*": "multiplication"}[op]

		case "5th Grade":
			ops := []string{"+", "-", "*", "/"}
			op = ops[rand.Intn(len(ops))]
			switch op {
			case "+":
				a, b = rand.Intn(90)+10, rand.Intn(90)+10
				answer = a + b
			case "-":
				a, b = rand.Intn(90)+10, rand.Intn(90)+10
				answer = a - b
			case "*":
				// a and b ≤ 15
				a, b = rand.Intn(15)+1, rand.Intn(15)+1
				answer = a * b
			case "/":
				// Divisor max 15, dividend up to 3 digits
				b = rand.Intn(15) + 1
				answer = rand.Intn(20) + 1
				a = b * answer // ensures whole number
			}
			text = fmt.Sprintf("%d %s %d = ?", a, op, b)
			opType = map[string]string{
				"+": "addition", "-": "subtraction", "*": "multiplication", "/": "division",
			}[op]

		case "Algebra":
			var format = rand.Intn(7)

			switch format {
			case 0: // x + n = m
				x := rand.Intn(41) - 20
				n := rand.Intn(10) + 1
				text = fmt.Sprintf("x + %d = %d. What is x?", n, x+n)
				answer = x
				opType = "algebra_addition"

			case 1: // x - n = m
				x := rand.Intn(41) - 20
				n := rand.Intn(5) + 1
				text = fmt.Sprintf("x - %d = %d. What is x?", n, x-n)
				answer = x
				opType = "algebra_subtraction"

			case 2: // n + x = m
				x := rand.Intn(41) - 20
				n := rand.Intn(10) + 1
				text = fmt.Sprintf("%d + x = %d. What is x?", n, x+n)
				answer = x
				opType = "algebra_addition"

			case 3: // n - x = m
				x := rand.Intn(41) - 20
				n := rand.Intn(10) + x
				text = fmt.Sprintf("%d - x = %d. What is x?", n, n-x)
				answer = x
				opType = "algebra_subtraction"

			case 4: // x * n = m
				x := rand.Intn(41) - 20
				n := rand.Intn(6) + 1
				text = fmt.Sprintf("x * %d = %d. What is x?", n, x*n)
				answer = x
				opType = "algebra_multiplication"

			case 5: // x / n = m
				n := rand.Intn(5) + 1   // still always positive
				m := rand.Intn(21) - 10 // [-10, 10]
				if m == 0 {
					m = 1 // avoid zero
				}
				text = fmt.Sprintf("x ÷ %d = %d. What is x?", n, m)
				answer = n * m
				opType = "algebra_division"

			case 6: // n * x = m
				x := rand.Intn(41) - 20
				n := rand.Intn(6) + 1
				text = fmt.Sprintf("%d * x = %d. What is x?", n, x*n)
				answer = x
				opType = "algebra_multiplication"
			}

		default:
			// fallback: simple addition
			a, b = rand.Intn(11), rand.Intn(11)
			op = "+"
			answer = a + b
			text = fmt.Sprintf("%d %s %d = ?", a, op, b)
			opType = "addition"
		}

		id := fmt.Sprintf("%s|%d", difficulty, rand.Int()) // prevent duplicates across runs
		if _, exists := used[id]; exists {
			continue
		}
		used[id] = struct{}{}

		q = Question{
			Text:     text,
			Answer:   answer,
			OpType:   opType,
			UniqueID: id,
		}
		break
	}

	return q
}
```

The generateQuestion helper function takes the selected difficulty, and the map of already generated questions 
and returns a type Question struct. All questions have a check to ensure integer outcomes. Difficulties are set as such:  
* 1st Grade - addition and subtraction, with a check to prevent negative answers. Numbers 1-9
* 3rd Grade - addition, subtraction, and simple multiplication, with a check to make sure that multiplication is only single digit by single digit. Add/Sub numbers -20 - 20, Mult 1-9
* 5th Grade - addition, subtraction, multiplication, and division. Add/Sub numbers -100 - 100, Mult/Div numbers 1-15
* Algebra - 6 states:
  * x - n = m
  * n + x = m 
  * n - x = m
  * x * n = m
  * x / n = m (always positive, never zero, no remainders)
  * n * x = m

It assigns a random id number to the question and ensures it doesn't already exist. If it passes
this check, the generated question is returned.  
If the difficulty is corrupted, the default is to ask a simple addition problem.

### Timers

```go
type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(time.Now())
	})
}

type flashDoneMsg struct{}

func flashTimeout() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return flashDoneMsg{}
	})
}

type flashFadeMsg struct{}

func flashFadeTick() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(t time.Time) tea.Msg {
		return flashFadeMsg{}
	})
}

type fuseTickMsg struct{}

func fuseTick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return fuseTickMsg{}
	})
}

type pulseTickMsg struct{}

func pulseTick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return pulseTickMsg{}
	})
}
```

There are five timers in use by the program. These are built-in timers native to BubbleTea, but work on the cycle "tick" principal.
In each function, you can set the amount of time that is elapsed during each tick. 
* tick() - main timer - 1 second intervals
* flashTimeout() - timer for flash message duration - 1 second intervals
* flashFadeTick() - timer for time bar color change flash - 300 millisecond intervals
* fuseTick() - timer for fuse animation - 150 millisecond intervals
* pulseTick() - timer for final 10 seconds, rapidly changes color of time bar - 150 millisecond intervals

### Styles

```go
var (
	greenBar    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	yellowBar   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	redBar      = lipgloss.NewStyle().Foreground(lipgloss.Color("88"))
	brightGreen = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	brightRed   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func renderCountdownBar(timeRemaining, totalTime int, width int, adjust int, flashColorOverride string, fuseChar string) string {
	if timeRemaining <= 0 {
		return "💥 TIME'S UP! 💥"
	}

	// % complete
	percent := float64(timeRemaining) / float64(totalTime)
	filled := int(percent * float64(width))
	filled += adjust
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	// Choose color
	var barColor lipgloss.Style
	if flashColorOverride != "" {
		switch flashColorOverride {
		case "brightGreen":
			barColor = brightGreen
		case "brightRed":
			barColor = brightRed
		}
	} else {
		switch {
		case percent > 0.5:
			barColor = greenBar
		case percent > 0.2:
			barColor = yellowBar
		default:
			barColor = redBar
		}
	}

	bar := ""
	for i := 0; i < width; i++ {
		if i == filled-1 {
			bar += lipgloss.NewStyle().
				Foreground(lipgloss.Color("11")).Render(fuseChar)
		} else if i < filled {
			bar += barColor.Render("█")
		} else {
			bar += " "
		}
	}

	return fmt.Sprintf("💣[%s]", bar)
}
```  

These variables and this function render the time bar. The bar is filled based on the percentage of time remaining,
and recalculate the size based on the user's performance in the quiz. This also controls the flash of color displayed after an
answer is submitted.