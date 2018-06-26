package main

import (
	"os"
	"io"
	"fmt"
	"time"
	"sync"
	"bufio"
	"errors"
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
	InstanceActionURL = "http://169.254.169.254/latest/meta-data/spot/instance-action"
	InstanceMetadataURLPattern = "http://169.254.169.254/latest/meta-data/%s"
	StartScriptPath = "/minecraft/ServerStart.sh"
)

var (
	logger *logrus.Logger
	ErrSpotTermination = errors.New("Spot instance terminating")
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

type InstanceAction struct {
	Action string `json:"action"`
	Time string `json:"time"`
}

func getTerminationTime() (*time.Time, error)  {
	resp, err := http.Get(InstanceActionURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Failed to retrieve instance action.  Status code: %d", resp.StatusCode)
	}

	payload, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var action InstanceAction
	err = json.Unmarshal(payload, &action)
	if err != nil {
		return nil, err
	}

	t, err := time.Parse(time.RFC3339, action.Time)
	if err != nil {
		return nil, err
	}

	return &t, nil
}

func attachEIP(ec2Client *ec2.EC2, params *MinecraftParams) error {
	instanceId, err := getInstanceMetadata("instance-id")
	if err != nil {
		return err
	}

	// This should succeed even if the address is already present.
	_, err = ec2Client.AssociateAddress(&ec2.AssociateAddressInput{
		AllocationId: aws.String(params.ElasticIP),
		InstanceId: aws.String(instanceId),
	})
	return err
}

func attachEBS(ec2Client *ec2.EC2, params *MinecraftParams) error {
	cmd := exec.Command("find-nvme-device", params.EBSVolume)
	err := cmd.Run()
	if err == nil {
		logger.Infof("EBS volume %s seems to be attached already.", params.EBSVolume)
	}

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

func detachEBS(ec2Client *ec2.EC2, params *MinecraftParams) error {
	cmd := exec.Command("find-nvme-device", params.EBSVolume)
	err := cmd.Run()
	if err == nil {
		logger.Infof("EBS volume %s seems to be attached already.", params.EBSVolume)
	}

	instanceId, err := getInstanceMetadata("instance-id")
	if err != nil {
		return err
	}

	_, err = ec2Client.DetachVolume(&ec2.DetachVolumeInput{
		Device: aws.String("/dev/sdf"),
		VolumeId: aws.String(params.EBSVolume),
		InstanceId: aws.String(instanceId),
	})
	return err
}

func mountEBS(params *MinecraftParams) error {
	cmd := exec.Command("find-nvme-device", params.EBSVolume)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("Failed to find NVMe device: %v", err)
	}
	dev := string(output)
	if dev[len(dev)-1] == '\n' {
		dev = dev[:len(dev)-1]
	}

	cmd = exec.Command("mounter", dev)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("Failed to mount EBS volume: %v", err)
	}

	return nil
}

func unmountEBS(params *MinecraftParams) error {
	cmd := exec.Command("unmounter")
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("Failed to unmount EBS volume: %v", err)
	}

	return nil
}

type Minecraft struct {
	params *MinecraftParams
	cmd *exec.Cmd
	stdinmut sync.Mutex
	stdin io.WriteCloser
	stdout io.ReadCloser
	quiesce uint64
}

func NewMinecraft(params *MinecraftParams) *Minecraft {
	return &Minecraft{
		params: params,
		stdin: nil,
		stdout: nil,
		quiesce: 0,
	}
}

func (self *Minecraft) Stop(quiesce bool) {
	self.stdinmut.Lock()
	defer self.stdinmut.Unlock()
	if quiesce {
		atomic.StoreUint64(&self.quiesce, 1)
	}
	if self.stdin != nil {
		self.stdin.Write([]byte("say The server will shut down in 60 seconds.  Get to safety now!\n"))
		time.Sleep(60 * time.Second)
		self.stdin.Write([]byte("stop\n"))
	}
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
		self.stdinmut.Lock()
		defer self.stdinmut.Unlock()
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

	if atomic.LoadUint64(&self.quiesce) == 0 {
		err = cmd.Run()
	}

	if atomic.LoadUint64(&self.quiesce) == 1 {
		return ErrSpotTermination
	} else {
		return err
	}
}

func cleanupDiskMounts(ec2Client *ec2.EC2, params *MinecraftParams) error {
	err := unmountEBS(params)
	if err != nil {
		return fmt.Errorf("Failed to unmount EBS volume: %v", err)
	}

	// Wait for things to settle
	time.Sleep(10 * time.Second)

	err = detachEBS(ec2Client, params)
	if err != nil {
		return fmt.Errorf("Failed to detach EBS volume: %v", err)
	}

	return nil
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

	// Wait for the volume to attach
	time.Sleep(10 * time.Second)

	err = mountEBS(params)
	if err != nil {
		logger.Fatalf("Failed to mount EBS volume: %v", err)
	}

	success := false
	var keepRunning uint64 = 1
	mc := NewMinecraft(params)

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
			mc.Stop(false)
		case syscall.SIGHUP:
			mc.Stop(false)
		}
	}()

	go func() {
		for {
			t, err := getTerminationTime()
			if err != nil {
				logger.Errorf("Failed to get termination time: %v", err)
			}
			if t != nil {
				now := time.Now()
				if now.Sub(*t) < 3 * time.Minute {
					mc.Stop(true)
					// After we've sent the stop command, we
					// don't need this goroutine anymore.
					break
				}
			}
			time.Sleep(5 * time.Second)
		}
	}()

	// TODO: can I express this in a cleaner way?
	needsCleanup := true

	for atomic.LoadUint64(&keepRunning) == 1 {
		err = mc.Run()
		if err == ErrSpotTermination {
			success = true
			cleanupErr := cleanupDiskMounts(ec2Client, params)
			if cleanupErr != nil {
				logger.Error(cleanupErr.Error())
			} else {
				needsCleanup = false
			}
			for {
				time.Sleep(5 * time.Second)
				if atomic.LoadUint64(&keepRunning) == 0 {
					break
				}
			}
		} else {
			success = err == nil
		}
	}

	if needsCleanup {
		cleanupErr := cleanupDiskMounts(ec2Client, params)
		if cleanupErr != nil {
			logger.Error(cleanupErr.Error())
		}
	}
	if !success {
		os.Exit(1)
	}
}
