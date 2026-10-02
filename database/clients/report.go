package clients

import (
	"fmt"
	"math"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	v2 "github.com/komari-monitor/komari/protocol/v2"
)

func GetClientUUIDByToken(token string) (clientUUID string, err error) {
	db := dbcore.GetDBInstance()
	var client models.Client
	err = db.Where("token = ?", token).First(&client).Error
	if err != nil {
		return "", err
	}
	return client.UUID, nil
}

// 检查数据防止异常数据导致数据库损坏
func ReportVerify(report v2.Report) error {
	if len(report.Message)>4096 || len(report.CPU.Name)>512 || len(report.CPU.Arch)>64 || len(report.Method)>64 { return fmt.Errorf("report string limit exceeded") }
	if report.GPU != nil {
		if report.GPU.Count<0 || report.GPU.Count>32 || len(report.GPU.DetailedInfo)>32 { return fmt.Errorf("GPU device limit exceeded") }
		if math.IsNaN(report.GPU.AverageUsage) || math.IsInf(report.GPU.AverageUsage,0) || report.GPU.AverageUsage<0 || report.GPU.AverageUsage>100 { return fmt.Errorf("invalid GPU utilization") }
		for _, device := range report.GPU.DetailedInfo { if len(device.Name)>256 || device.MemoryTotal<0 || device.MemoryUsed<0 || math.IsNaN(device.Utilization) || math.IsInf(device.Utilization,0) || device.Utilization<0 || device.Utilization>100 { return fmt.Errorf("invalid GPU device") } }
	}
	if err := report.DiskIO.Validate(); err != nil {
		return err
	}
	// 防止输入不合理范围
	if report.CPU.Usage < 0 || report.CPU.Usage > 100 {
		return fmt.Errorf("CPU.Usage must be between 0 and 100")
	}

	if report.Load.Load1 < 0 || report.Load.Load1 > 1000 {
		return fmt.Errorf("Load.Load1 must be non-negative, got %.2f", report.Load.Load1)
	}

	checkFloat64 := func(name string, val float64) error {
		if math.IsNaN(val) || math.IsInf(val,0) || val > math.MaxFloat64-1 || val < -math.MaxFloat64+1 {
			return fmt.Errorf("%s value exceeds float64 range: %g", name, val)
		}
		return nil
	}

	// [float64] 防止数据溢出
	if err := checkFloat64("CPU.Usage", report.CPU.Usage); err != nil {
		return err
	}
	if err := checkFloat64("Load.Load1", report.Load.Load1); err != nil {
		return err
	}
	for _, val := range []float64{report.Load.Load5, report.Load.Load15} { if err := checkFloat64("Load",val); err != nil { return err }; if val<0 || val>1000 { return fmt.Errorf("invalid load") } }

	checkInt64 := func(name string, val int64) error {
		if val < 0 {
			return fmt.Errorf("%s must be non-negative, got %d", name, val)
		}
		if val > math.MaxInt64-1 {
			return fmt.Errorf("%s exceeds int64 max limit: %d", name, val)
		}
		return nil
	}

	// [int64] 防止数据溢出
	// Ram 验证
	if err := checkInt64("Ram.Used", report.Ram.Used); err != nil {
		return err
	}
	if err := checkInt64("Ram.Total", report.Ram.Total); err != nil {
		return err
	}
	// Swap 验证
	if err := checkInt64("Swap.Used", report.Swap.Used); err != nil {
		return err
	}
	if err := checkInt64("Swap.Total", report.Swap.Total); err != nil {
		return err
	}
	// Disk 验证
	if err := checkInt64("Disk.Used", report.Disk.Used); err != nil {
		return err
	}
	if err := checkInt64("Disk.Total", report.Disk.Total); err != nil {
		return err
	}
	// Network 验证
	if err := checkInt64("Network.Up", report.Network.Up); err != nil {
		return err
	}
	if err := checkInt64("Network.Down", report.Network.Down); err != nil {
		return err
	}
	if err := checkInt64("Network.TotalUp", report.Network.TotalUp); err != nil {
		return err
	}
	if err := checkInt64("Network.TotalDown", report.Network.TotalDown); err != nil {
		return err
	}
	// 拒绝所有负数Int
	if report.Process < 0 {
		return fmt.Errorf("Process must be non-negative: %d", report.Process)
	}
	if report.Connections.TCP < 0 {
		return fmt.Errorf("Connections.TCP must be non-negative: %d", report.Connections.TCP)
	}
	if report.Connections.UDP < 0 {
		return fmt.Errorf("Connections.UDP must be non-negative: %d", report.Connections.UDP)
	}
	return nil
}
