package service

import (
	"fmt"
	"log"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

const (
	// DefaultProfile is the default AWS Profile
	DefaultProfile = "default"
)

// Config holds service settings
type Config struct {
	Profile *string
	Args    []string
}

// Service holds internal state
type Service struct {
	config    *Config
	svc       *ec2.EC2
	app       *tview.Application
	table     *tview.Table
	instances map[string]*Instance
}

// NewService returns a new service instance
func NewService(conf *Config) *Service {
	app := tview.NewApplication()

	table := tview.NewTable().
		SetFixed(1, 0).
		SetSelectable(true, false)
	table.SetBorderPadding(0, 0, 1, 1)

	app.SetRoot(table, true)

	if conf == nil {
		p := DefaultProfile
		conf = &Config{
			Profile: &p,
		}
	}

	s := Service{
		config:    conf,
		app:       app,
		table:     table,
		instances: make(map[string]*Instance),
	}

	return &s
}

// Run starts the application
func (s *Service) Run() {
	s.table.SetSelectedFunc(s.handleSelected)
	s.ec2svc()
	s.fetchInstances()
	s.updateTable()

	s.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'r', 'R':
			s.fetchInstances()
			s.updateTable()
			return nil

			// case 's':
			// 	row, col := s.table.GetSelection()
			// 	s.handleSelected(row, col)
			// 	return nil
		}

		return event
	})

	// for _, i := range s.instances {
	// 	fmt.Printf("%+v\n", i)
	// }
	s.app.Run()
}

func (s *Service) handleSelected(row int, col int) {
	cell := s.table.GetCell(row, col)
	ref := cell.GetReference()
	instance, ok := s.instances[ref.(string)]

	if ok && len(instance.PrivateIP) > 0 {
		s.app.Suspend(func() {
			instance.runSSH()
		})
	}
}

func (s *Service) ec2svc() {
	creds := credentials.NewChainCredentials(
		[]credentials.Provider{
			// &credentials.EnvProvider{},
			&credentials.SharedCredentialsProvider{
				Profile: *s.config.Profile,
			},
		},
	)

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Credentials: creds},
		Profile:           *s.config.Profile,
	}))

	// s.svc = ec2.New(session.Must(session.NewSession(conf)))
	s.svc = ec2.New(sess)
}

func (s *Service) fetchInstances() {
	res, err := s.svc.DescribeInstances(&ec2.DescribeInstancesInput{})
	if err != nil {
		log.Fatalln(err.Error())
	}

	insts := map[string]*Instance{}
	for _, reservation := range res.Reservations {
		for _, instance := range reservation.Instances {
			i := NewInstance(instance)
			insts[i.ID] = i
		}
	}
	s.instances = insts
}

func (s *Service) updateTable() {
	tagsNames := selectedTags(s.instances)
	tagsCount := len(tagsNames)
	headers := []string{"ID", "Private IP", "Public IP", "State", "AZ", "Type", "AMI", "Running"}
	row := 0

	insts := []*Instance{}
	for _, i := range s.instances {
		insts = append(insts, i)
	}
	sort.Sort(InstanceSort(insts))

	for _, instance := range insts {
		color := "[white]"
		switch instance.State {
		case "terminated":
			color = "[grey]"
		case "pending", "stopping", "shutting-down":
			color = "[orange]"
		}

		tags := instance.TagValues(tagsNames)
		vals := []string{
			instance.ID,
			instance.PrivateIP,
			instance.PublicIP,
			instance.State,
			instance.AZ,
			instance.Type,
			instance.AMI,
			instance.RunningSince(),
		}
		values := append(tags, vals...)

		// render headers
		if row == 0 {
			for c, t := range tagsNames {
				tag := tview.NewTableCell("Tag:" + t).
					SetSelectable(false)
				s.table.SetCell(0, c, tag)
			}

			for c, h := range headers {
				head := tview.NewTableCell(h).
					SetSelectable(false)
				s.table.SetCell(0, c+tagsCount, head)
			}

			row++
		}

		for col, val := range values {
			cell := tview.NewTableCell(fmt.Sprintf("%s%s[-:-:-]", color, val)).
				SetSelectable(true).
				SetReference(instance.ID)
			s.table.SetCell(row, col, cell)
		}

		row++
	}
}
