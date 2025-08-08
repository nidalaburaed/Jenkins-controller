package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	jenkinsURL     = "http://localhost:8080"
	jenkinsUser    = "nokiaadmin"
	jenkinsAPIToken = "API-key"
	backupDir      = "/backup/jenkins_home"
	targetDir      = "/var/lib/jenkins"
)

func runCommand(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error running %s %v: %v\n", name, args, err)
	}
	return string(out)
}

func startJenkins() {
	fmt.Println("Starting Jenkins service...")
	status := runCommand("systemctl", "is-active", "--quiet", "jenkins")
	if status == "" {
		runCommand("sudo", "systemctl", "start", "jenkins")
		time.Sleep(5 * time.Second)
		fmt.Println(runCommand("sudo", "systemctl", "status", "jenkins"))
	} else {
		fmt.Println("Jenkins is already running.")
	}

	fmt.Println("Checking if Jenkins is accessible on port 8080...")
	err := exec.Command("nc", "-zv", "localhost", "8080").Run()
	if err == nil {
		fmt.Println("Jenkins is reachable at http://localhost:8080")
	} else {
		fmt.Println("Jenkins is not listening on port 8080")
	}
}

func restoreBackup() {
	fmt.Printf("Restoring Jenkins home from %s to %s...\n", backupDir, targetDir)
	runCommand("sudo", "systemctl", "stop", "jenkins")
	runCommand("sudo", "cp", "-r", backupDir+"/.", targetDir)
	runCommand("sudo", "chown", "-R", "jenkins:jenkins", targetDir)
	fmt.Println("Restarting Jenkins after restore...")
	runCommand("sudo", "systemctl", "start", "jenkins")
}

func triggerJobs() {
	jobs := []string{"job1", "job2"}
	fmt.Println("Triggering smoke test jobs...")
	for _, job := range jobs {
		url := fmt.Sprintf("%s/job/%s/build", jenkinsURL, job)
		req, _ := http.NewRequest("POST", url, nil)
		req.SetBasicAuth(jenkinsUser, jenkinsAPIToken)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("Error triggering job %s: %v\n", job, err)
			continue
		}
		fmt.Printf("Triggered job: %s (HTTP %d)\n", job, resp.StatusCode)
		time.Sleep(2 * time.Second)
	}
}

func checkAgents() {
	fmt.Println("Checking Jenkins agents status...")
	url := fmt.Sprintf("%s/computer/api/json", jenkinsURL)
	req, _ := http.NewRequest("GET", url, nil)
	req.SetBasicAuth(jenkinsUser, jenkinsAPIToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("Failed to get agents:", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Computer []struct {
			DisplayName string `json:"displayName"`
		} `json:"computer"`
	}
	json.Unmarshal(body, &result)

	for _, comp := range result.Computer {
		agent := comp.DisplayName
		statusURL := fmt.Sprintf("%s/computer/%s/api/json", jenkinsURL, agent)
		req, _ := http.NewRequest("GET", statusURL, nil)
		req.SetBasicAuth(jenkinsUser, jenkinsAPIToken)
		resp, _ := http.DefaultClient.Do(req)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		var statusResult struct {
			Offline bool `json:"offline"`
		}
		json.Unmarshal(body, &statusResult)

		if statusResult.Offline {
			fmt.Printf("Agent %s is offline. Attempting to reconnect...\n", agent)
			relaunchURL := fmt.Sprintf("%s/computer/%s/relaunch", jenkinsURL, agent)
			req, _ := http.NewRequest("POST", relaunchURL, nil)
			req.SetBasicAuth(jenkinsUser, jenkinsAPIToken)
			http.DefaultClient.Do(req)
		} else {
			fmt.Printf("Agent %s is online.\n", agent)
		}
	}
}

func statusReport() {
	fmt.Println("=== Jenkins Instance Status Report ===")
	fmt.Println("Uptime:")
	fmt.Print(runCommand("uptime"))

	fmt.Println("\nDisk usage (/var/lib/jenkins):")
	fmt.Print(runCommand("df", "-h"))

	fmt.Println("\nLast 5 lines of Jenkins log:")
	fmt.Print(runCommand("sudo", "tail", "-n", "5", "/var/log/jenkins/jenkins.log"))

	fmt.Println("\nRunning Jenkins processes:")
	out := runCommand("ps", "aux")
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "jenkins") && !strings.Contains(line, "grep") {
			fmt.Println(line)
		}
	}

	fmt.Println("\nJenkins listening on port 8080?")
	err := exec.Command("nc", "-z", "localhost", "8080").Run()
	if err == nil {
		fmt.Println("Port 8080 is open")
	} else {
		fmt.Println("Port 8080 is not open")
	}
}

func main() {
	startJenkins()
	restoreBackup()
	triggerJobs()
	checkAgents()
	statusReport()
}