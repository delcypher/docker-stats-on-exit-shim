// Copyright 2016 Dan Liew
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
  "encoding/json"
  "fmt"
  "os"
  "os/exec"
  "os/signal"
  "syscall"
  "time"
  "strconv"
  cgroups "github.com/opencontainers/runc/libcontainer/cgroups"
  cgroups_fs "github.com/opencontainers/runc/libcontainer/cgroups/fs"
)

func printUsage() {
  fmt.Printf("%s <command> [arg...]\n", os.Args[0])
  fmt.Println("environment variables STATS_OUTPUT_FILE=/dev/stdout STATS_OUTPUT_MINIFIED=false STATS_OUTPUT_PREFIX=")
  fmt.Println("Runs <command> and on termination outputs cgroup usage information")
}

type Stats struct {
  WallClockTime int64 `json:"wall_time"`
  UserCPUTime int64 `json:"user_cpu_time"`
  SysCPUTime int64 `json:"sys_cpu_time"`
  Cgroups *cgroups.Stats `json:"cgroups"`
}

const (
  FailExitCode = 1
)

var signalsToForward = []os.Signal {
  // Unfortunately we can't forward SIGKILL or SIGSTOP
  syscall.SIGCONT,
  syscall.SIGHUP,
  syscall.SIGINT,
  syscall.SIGPROF,
  syscall.SIGQUIT,
  syscall.SIGTERM,
  syscall.SIGUSR1,
  syscall.SIGUSR2,
}


func fail(template string, args ...interface{}) {
  msg := os.Args[0] + ": " + template
  fmt.Fprintf(os.Stderr, msg, args...)
  os.Exit(FailExitCode)
}

func getEnv(key, defaultValue string) string {
  value := os.Getenv(key)
  if len(value) == 0 {
    return defaultValue
  }
  return value
}

func main() {
  exitCode := 0;

  if len(os.Args) < 2 {
    printUsage()
    os.Exit(1)
  }

  // Open file for writing stats
  f, err := os.Create(getEnv("STATS_OUTPUT_FILE", "/dev/stdout"))
  if err != nil {
    fail("Failed to create stats file %s: %s\n", os.Args[1], err)
  }
  defer f.Close()

  // Find all the cgroup subsystems
  subsystems, err := cgroups.GetAllSubsystems()
  if err != nil {
    fail("Failed to retrieve cgroup subsystem: %s\n", err)
  }

  subsystemToPathMap := make(map[string]string)

  // Find where those subsystems are mounted
  for _ , name := range subsystems {
    // HACK: Skip `pids` subsystem if the file we need doesn't exist.
    if name == "pids" {
      if _, err := os.Stat("/sys/fs/cgroup/pids/pids.current"); os.IsNotExist(err) {
        continue
      }
    }
    path, err := cgroups.FindCgroupMountpoint(name)
    if err != nil {
      fail("Failed to get path for cgroup %s: %s\n", name, err)
    }
    //fmt.Printf("Found %s with path %s\n", name, path)
    subsystemToPathMap[name] = path
  }

  // Make a fake Cgroup manager
  // FIXME: We're assuming cgroupV1 layout here. We should
  // have some sort of configuration time option to choose
  // what to use.
  manager := cgroups_fs.Manager{ Paths:subsystemToPathMap }


  // Run the subproccess
  cmd := exec.Command(os.Args[1], os.Args[2:]...)
  cmd.Stdin = os.Stdin
  cmd.Stdout = os.Stdout
  cmd.Stderr = os.Stderr
  cmd.Env = nil // Use the environment of the current process.

  // Setup signal handling forwarding.
  signalChan := make(chan os.Signal, 1)
  signal.Notify(signalChan, signalsToForward...)
  go func() {
    for {
      // Receive the signal and forward to the process
      signal, notClosed := <-signalChan
      if !notClosed {
        // Chanel has been closed. There are no more signals to
        // forward.
        break
      }
      signalSendErr := cmd.Process.Signal(signal)
      // FIXME: We should optionally log this information to a file.
      //fmt.Printf("Forwarding %v to PID %v\n", signal, cmd.Process.Pid)
      if signalSendErr != nil {
        // fmt.Printf("Failed to send signal: %s", signalSendErr)
      }
    }
  }()

  // FIXME: This is the wrong way to measure wall-clock time
  // as it is sensitive to system clock adjustments.
  wallclockStart := time.Now()
  exit := cmd.Run()
  wallclockElapsed := time.Since(wallclockStart)
  stats, err := manager.GetStats()
  close(signalChan)

  if err != nil {
    fail("Failed to retrieve stats: %s\n", err)
  }

  if exit != nil {
    if exitError, rightType := exit.(*exec.ExitError); rightType {
      // The command exited with a non-zero exit code
      status := exitError.Sys().(syscall.WaitStatus)
      exitCode = status.ExitStatus()
    } else {
      fail("Failed to run command: %v\n", exit)
    }
  }

  combinedStats := Stats{
    // Golang's docs claim the user and sys time are for
    // the process and all its children.
    WallClockTime: wallclockElapsed.Nanoseconds(),
    UserCPUTime: cmd.ProcessState.UserTime().Nanoseconds(),
    SysCPUTime: cmd.ProcessState.SystemTime().Nanoseconds(),
    Cgroups: stats,
  }

  //fmt.Printf("Stats: %+v", combinedStats)
  statsPrefix := getEnv("STATS_OUTPUT_PREFIX", "")
  minified := false
  minified, _ = strconv.ParseBool(getEnv("STATS_OUTPUT_MINIFIED", "false"))
  var statsAsBytes []byte
  if minified {
    statsAsBytes, err = json.Marshal(&combinedStats)
  } else {
    statsAsBytes, err = json.MarshalIndent(&combinedStats, "", "  ")
  }

  if err != nil {
    fail("Failed to serialize stats to JSON: %s\n", err)
  }

  _, err = f.Write([]byte(statsPrefix))
  _, err = f.Write(statsAsBytes)
  if err != nil {
    fail("Failed to write stats to file: %s\n", err)
  }

  os.Exit(exitCode);
}
