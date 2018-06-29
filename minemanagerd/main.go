package main

import (
	"os"
	"io"
	"fmt"
	"time"
	"sync"
	"bufio"
	"errors"
	"strings"
	"sync/atomic"
	"os/exec"
	"syscall"
	"net/http"
	"io/ioutil"
	"os/signal"
	"encoding/json"

	"github.com/sirupsen/logrus"

	aws "github.com/aws/aws-sdk-go/aws"
	awserr "github.com/aws/aws-sdk-go/aws/awserr"
	session "github.com/aws/aws-sdk-go/aws/session"
	ec2 "github.com/aws/aws-sdk-go/service/ec2"
	s3 "github.com/aws/aws-sdk-go/service/s3"
	"github.com/kdar/logrus-cloudwatchlogs"
)

const (
	UserDataURL = "http://169.254.169.254/latest/user-data"
	InstanceActionURL = "http://169.254.169.254/latest/meta-data/spot/instance-action"
	InstanceMetadataURLPattern = "http://169.254.169.254/latest/meta-data/%s"
	StartScriptPath = "/minecraft/ServerStart.sh"
	MinecraftDir = "/minecraft"
	PersistentConfigDir = "/ebs/spotcraft"
	OpsFile = "/ops.json"
	PropsFile = "/server.properties"
	WhitelistFile = "/whitelist.json"
	BannedPlayersFile = "/banned-players.json"
	BannedIPsFile = "/banned-ips.json"
)

var (
	ec2Client *ec2.EC2
	s3Client *s3.S3
	logger *logrus.Logger
	ErrSpotTermination = errors.New("Spot instance terminating")
	ErrCantFindSelf = errors.New("Failed to find self in EC2.  Wrong/missing IAM instance profile?")
	ErrNoSuchServerFile = errors.New("No such server file")
	DefaultFiles = []string{ OpsFile, PropsFile, WhitelistFile, BannedPlayersFile, BannedIPsFile }
)

type MinecraftParams struct {
	ElasticIP string `json:"elastic_ip"`
	EBSVolume string `json:"ebs_volume"`
	ConfigBucket string `json:"config_bucket"`
	TagName string `json:"tag_name"`
	LogGroup *string `json:"log_group"`
	LogStream *string `json:"log_stream"`
	MaxRam *string `json:"max_ram"`
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

func (self *MinecraftParams) GetServerFile(path string) ([]byte, error) {
	instanceId, err := getInstanceMetadata("instance-id")
	if err != nil {
		return nil, err
	}

	ec2Resp, err := ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{ &instanceId },
	})
	if err != nil {
		return nil, err
	}

	if len(ec2Resp.Reservations) == 0 {
		return nil, ErrCantFindSelf
	}
	resv := ec2Resp.Reservations[0]
	if len(resv.Instances) == 0 {
		return nil, ErrCantFindSelf
	}

	serverName := ""
	inst := resv.Instances[0]
	for _, tag := range inst.Tags {
		if tag.Key != nil && *tag.Key == self.TagName {
			if tag.Value == nil {
				return nil, fmt.Errorf("Invalid value for %s tag", self.TagName)
			}
			serverName = *tag.Value
			break
		}
	}

	s3Resp, err := s3Client.GetObject(&s3.GetObjectInput{
		Bucket: &self.ConfigBucket,
		Key: aws.String(fmt.Sprintf("servers/%s%s", serverName, path)),
	})
	if err != nil {
		// WTF is this, Amazon?
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == s3.ErrCodeNoSuchKey {
				return nil, ErrNoSuchServerFile
			}
		}
		return nil, err
	}
	defer s3Resp.Body.Close()

	return ioutil.ReadAll(s3Resp.Body)
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

func attachEIP(params *MinecraftParams) error {
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

func attachEBS(params *MinecraftParams) error {
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

func detachEBS(params *MinecraftParams) error {
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

func (self *Minecraft) copyConfigOverrides() error {
	copier := func(remoteDir, localDir string, overwrite bool) func(filepath string) error {
		return func(filepath string) error {
			localpath := localDir + filepath
			if _, err := os.Stat(localpath); os.IsNotExist(err) || overwrite {
				content, err := self.params.GetServerFile(remoteDir + filepath)
				if err != nil {
					return err
				}
				var f *os.File
				if overwrite {
					f, err = os.OpenFile(
						localpath,
						os.O_WRONLY | os.O_CREATE | os.O_TRUNC,
						0644,
					)
				} else {
					f, err = os.Create(localpath)
				}
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = f.Write(content)
				if err != nil {
					return err
				}
			}
			
			return nil
		}
	}
	// Defaults should only be copied if they don't exist.
	// These are files like ops.json and server.properties.
	copyDefault := copier("defaults", PersistentConfigDir, false)
	// Overrides may be changed in S3 and should always overwrite
	// whatever may be present locally.
	// These are typically mod-specific config files.
	copyOverride := copier("overrides", MinecraftDir, true)

	for _, filename := range DefaultFiles {
		err := copyDefault(filename)
		if err != nil {
			return err
		}
	}

	// overrides.lst is a simple text file with one path (relative to /minecraft)
	// per line.  There must be a file in the S3 bucket with the same path
	// relative to BUCKET/servers/$serverName/overrides.
	overridesData, err := self.params.GetServerFile("/overrides.lst")
	if err != nil {
		if err == ErrNoSuchServerFile {
			return nil
		}
		return err
	}

	for _, override := range strings.Split(string(overridesData), "\n") {
		err := copyOverride(override)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *Minecraft) Run() error {
	err := self.copyConfigOverrides()
	if err != nil {
		return err
	}
	cmd := exec.Command(StartScriptPath)

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

func cleanupDiskMounts(params *MinecraftParams) error {
	err := unmountEBS(params)
	if err != nil {
		return fmt.Errorf("Failed to unmount EBS volume: %v", err)
	}

	// Wait for things to settle
	time.Sleep(10 * time.Second)

	err = detachEBS(params)
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
	ec2Client = ec2.New(sess)
	s3Client = s3.New(sess)

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

	err = attachEIP(params)
	if err != nil {
		logger.Fatalf("Failed to attach EIP: %v", err)
	}

	err = attachEBS(params)
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
			cleanupErr := cleanupDiskMounts(params)
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
		cleanupErr := cleanupDiskMounts(params)
		if cleanupErr != nil {
			logger.Error(cleanupErr.Error())
		}
	}
	if !success {
		os.Exit(1)
	}
}
