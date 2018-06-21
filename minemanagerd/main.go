package main

import (
	"os"
	"io"
	"fmt"
	"time"
	"bufio"
	"sync/atomic"
	"os/exec"
	"syscall"
	"net/http"
	"io/ioutil"
	"os/signal"
	"encoding/json"

	"github.com/sirupsen/logrus"

	aws "github.com/aws/aws-sdk-go/aws"
	session "github.com/aws/aws-sdk-go/aws/session"
	ec2 "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kdar/logrus-cloudwatchlogs"
)

const (
	UserDataURL = "http://169.254.169.254/latest/user-data"
	TerminationTimeURL = "http://169.254.169.254/latest/meta-data/spot/termination-time"
	InstanceMetadataURLPattern = "http://169.254.169.254/latest/meta-data/%s"
	StartScriptPath = "/minecraft/ServerStart.sh"
)

var (
	logger *logrus.Logger
)

type MinecraftParams struct {
	ElasticIP string `json:"elastic_ip"`
	EBSVolume string `json:"ebs_volume"`
	LogGroup *string `json:"log_group"`
	LogStream *string `json:"log_stream"`
	ServerProperties *string `json:"server_properties"`
	OpList string `json:"op_list"`
}

func getMinecraftParams() (*MinecraftParams, error)  {
	resp, err := http.Get(UserDataURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Failed to retrieve userdata.  Status code: %d", resp.StatusCode)
	}

	payload, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var params MinecraftParams
	err = json.Unmarshal(payload, &params)
	if err != nil {
		return nil, err
	}

	return &params, nil
}

func getInstanceMetadata(name string) (string, error)  {
	resp, err := http.Get(fmt.Sprintf(InstanceMetadataURLPattern, name))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", nil
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Failed to retrieve %s.  Status code: %d", name, resp.StatusCode)
	}

	payload, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(payload), nil
}

func attachEIP(ec2Client *ec2.EC2, params *MinecraftParams) error {
	instanceId, err := getInstanceMetadata("instance-id")
	if err != nil {
		return err
	}

	_, err = ec2Client.AssociateAddress(&ec2.AssociateAddressInput{
		AllocationId: aws.String(params.ElasticIP),
		AllowReassociation: aws.Bool(false),
		InstanceId: aws.String(instanceId),
	})
	return err
}

func attachEBS(ec2Client *ec2.EC2, params *MinecraftParams) error {
	instanceId, err := getInstanceMetadata("instance-id")
	if err != nil {
		return err
	}

	_, err = ec2Client.AttachVolume(&ec2.AttachVolumeInput{
		Device: aws.String("/dev/sdf"),
		VolumeId: aws.String(params.EBSVolume),
		InstanceId: aws.String(instanceId),
	})
	return err
}

type Minecraft struct {
	params *MinecraftParams
	cmd *exec.Cmd
	stdin io.WriteCloser
	stdout io.ReadCloser
}

func NewMinecraft(params *MinecraftParams) *Minecraft {
	return &Minecraft{
		params: params,
		stdin: nil,
		stdout: nil,
	}
}

func (self *Minecraft) Stop() {
	self.stdin.Write([]byte("say The server will shut down in 60 seconds.  Get to safety now!\n"))
	time.Sleep(60 * time.Second)
	self.stdin.Write([]byte("stop\n"))
}

func (self *Minecraft) Run() error {
	cmd := exec.Command(StartScriptPath)

	var err error
	self.stdin, err = cmd.StdinPipe()
	if err != nil {
		return err
	}

	self.stdout, err = cmd.StdoutPipe()
	if err != nil {
		return err
	}

	defer func() {
		self.stdin.Close()
		self.stdout.Close()
		self.stdin = nil
		self.stdout = nil
	}()

	go func() {
		scanner := bufio.NewScanner(self.stdout)
		for scanner.Scan() {
			line := scanner.Text()
			logger.Info(line)
		}
	}()

	return cmd.Run()
}

func main() {
	region, err := getInstanceMetadata("placement/availability-zone")
	if err != nil {
		logrus.Fatalf("Failed to get instance availability zone: %v", err)
	}
	region = region[:len(region)-1]

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		logrus.Fatalf("Failed to create AWS session: %v", err)
	}
	ec2Client := ec2.New(sess)

	instanceId, err := getInstanceMetadata("instance-id")
	if err != nil {
		logrus.Fatalf("Failed to get instance ID: %v", err)
	}

	params, err := getMinecraftParams()
	if err != nil {
		logrus.Fatalf("Failed to get Minecraft parameters: %v", err)
	}

	logger = logrus.New()
	if params.LogGroup != nil && params.LogStream != nil {
		cwhook, err := logrus_cloudwatchlogs.NewHook(
			*params.LogGroup,
			*params.LogStream,
			&aws.Config{
				Region: aws.String(region),
			},
		)
		if err != nil {
			logrus.Fatalf("Failed to create CloudWatch logger: %v", err)
		}
		logger.Hooks.Add(cwhook)
		logger.Formatter = logrus_cloudwatchlogs.NewProdFormatter(
			logrus_cloudwatchlogs.AppName("minecraft"),
			logrus_cloudwatchlogs.Hostname(instanceId),
		)
	} else {
		logger.Info("Logger initialized!")
	}

	err = attachEIP(ec2Client, params)
	if err != nil {
		logger.Fatalf("Failed to attach EIP: %v", err)
	}

	err = attachEBS(ec2Client, params)
	if err != nil {
		logger.Fatalf("Failed to attach EBS volume: %v", err)
	}

	success := false
	var keepRunning uint64 = 1
	for atomic.LoadUint64(&keepRunning) == 1 {
		mc := NewMinecraft(params)
		if err != nil {
			logger.Fatalf("Failed to create Minecraft manager: %v", err)
		}

		sigc := make(chan os.Signal, 2)
		signal.Notify(sigc, syscall.SIGQUIT, syscall.SIGHUP)
		defer func() {
			signal.Stop(sigc)
			close(sigc)
		}()

		go func() {
			sig := <-sigc
			switch sig {
			case syscall.SIGQUIT:
				atomic.StoreUint64(&keepRunning, 0)
				mc.Stop()
			case syscall.SIGHUP:
				mc.Stop()
			}
		}()

		err = mc.Run()
		success = err == nil
	}

	if !success {
		os.Exit(1)
	}
}
