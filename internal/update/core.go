package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/utils/shutdown"
)

func UpdateCore() error {
	log.Infof("start update core")

	filename, err := getDownloadFilename()
	if err != nil {
		log.Warnf("update core failed: %v", err)
		return err
	}

	downloadUrl := updateUrl + "/" + filename
	log.Infof("download url: %s", downloadUrl)
	data, err := doRequestWithFallback(downloadUrl)
	if err != nil {
		log.Warnf("download failed: %v", err)
		return err
	}

	execPath, err := os.Executable()
	if err != nil {
		log.Warnf("get executable path failed: %v", err)
		return err
	}

	// 先将旧二进制重命名，避免 Linux 上 "text file busy" 导致无法覆盖
	oldPath := execPath + ".old"
	if err := os.Rename(execPath, oldPath); err != nil {
		log.Warnf("rename old binary failed: %v", err)
		// 非致命错误，继续尝试直接覆盖
	} else {
		log.Infof("renamed old binary to %s", oldPath)
	}

	if err := unzip(data, filepath.Dir(execPath)); err != nil {
		log.Warnf("unzip failed: %v", err)
		// 解压失败时尝试恢复旧二进制
		if _, statErr := os.Stat(oldPath); statErr == nil {
			os.Rename(oldPath, execPath)
		}
		return err
	}

	// 清理旧二进制
	os.Remove(oldPath)

	log.Infof("update core success")
	go restartExecutable(execPath)
	return nil
}

func getDownloadFilename() (string, error) {
	arch := runtime.GOARCH
	goos := runtime.GOOS

	switch goos {
	case "windows":
		switch arch {
		case "386":
			return "octopus-windows-x86.zip", nil
		case "amd64":
			return "octopus-windows-x86_64.zip", nil
		}
	case "darwin":
		switch arch {
		case "amd64":
			return "octopus-darwin-x86_64.zip", nil
		case "arm64":
			return "octopus-darwin-arm64.zip", nil
		}
	case "linux":
		switch arch {
		case "386":
			return "octopus-linux-x86.zip", nil
		case "amd64":
			return "octopus-linux-x86_64.zip", nil
		case "arm":
			return "octopus-linux-armv7.zip", nil
		case "arm64":
			return "octopus-linux-arm64.zip", nil
		}
	}
	return "", fmt.Errorf("unsupported platform: %s/%s", goos, arch)
}

func restartExecutable(execPath string) {
	shutdown.Shutdown()

	log.Infof("restarting: %q %q", execPath, os.Args[1:])

	if runtime.GOOS == "windows" {
		cmd := exec.Command(execPath, os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			log.Errorf("restarting failed: %v", err)
		}
		os.Exit(0)
	}

	if err := syscall.Exec(execPath, os.Args, os.Environ()); err != nil {
		log.Errorf("restarting failed: %v", err)
	}
}
