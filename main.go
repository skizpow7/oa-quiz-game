package main

import (
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type appState int

const (
	stateMenu appState = iota
	stateTimeSelect
	stateRunning
	stateResults

	clearScreen = "\033[H\033[2J"
)

type difficultyItem struct {
	name string
	desc string
}

func (d difficultyItem) Title() string       { return d.name }
func (d difficultyItem) Description() string { return d.desc }
func (d difficultyItem) FilterValue() string { return d.name }

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
	return m, nil
}

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

	default:
		return clearScreen + "Unknown state"
	}
}

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

// styles
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

func main() {
	p := tea.NewProgram(initialModel())
	if err := p.Start(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
