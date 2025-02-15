package clear

import (
	"db_backup_go/common"
	"db_backup_go/logging"
	"db_backup_go/modules/send"
	"fmt"
	"io/fs"
	"os"

	"github.com/pkg/sftp"
)

type Clear interface {
	ClearLocal(dict string) error
	ClearRemote(dict string) error
}

type backupFile struct {
	common.ConnInfo
	saveDay int
	dbList  *[]string
}

// 初始化旧备份清理，传入保存的天数和远端服务器连接信息（ConnInfo结构体）
func NewBackupClear(saveDay int, dbList *[]string, sc common.ConnInfo) *backupFile {
	return &backupFile{
		ConnInfo: sc,
		saveDay:  saveDay,
		dbList:   dbList,
	}
}

// 清理本地旧备份文件，传入本地路径，返回error
func (bf *backupFile) ClearLocal(dict string) error {
	//确认要保留的文件
	fsDict, err := os.ReadDir(dict)
	if err != nil {
		return fmt.Errorf("读取目录失败：%w", err)
	}
	var fsNameList []string
	for _, fsName := range fsDict {
		if fsName.IsDir() {
			fsNameList = append(fsNameList, fsName.Name())
		}
	}

	var backupPath []fs.DirEntry
	for _, v := range fsNameList {
		isContinue := false
		for index, dbName := range *bf.dbList {
			if index > len(*bf.dbList) || v == dbName {
				isContinue = false
				break
			}
			isContinue = true
		}

		if isContinue {
			continue
		}

		backupPath, err = os.ReadDir(dict + "/" + v)
		if err != nil {
			return fmt.Errorf("读取目录下文件失败：%w", err)
		}

		cf := common.SortByTime(backupPath)

		delDay := bf.saveDay
		if len(cf) < bf.saveDay {
			delDay = len(cf)
		}

		//排除大小为0的备份文件
		emptyFile := 0
		for index, f := range cf {
			if index == delDay {
				break
			}

			fbyte, err := os.ReadFile(dict + "/" + v + "/" + f.Name())
			if err != nil {
				return err
			}

			if len(fbyte) == 0 {
				emptyFile += 1
			}
		}

		delDay = delDay + emptyFile

		cf = cf[delDay:]

		//删除旧备份
		for _, oldfile := range cf {
			err := os.Remove(dict + "/" + v + "/" + oldfile.Name())
			if err != nil {
				return fmt.Errorf("旧备份文件删除失败：%w", err)
			}
		}

		//检查是否还存在指定份数的备份
		fsDict, err := os.ReadDir(dict + v)
		if err != nil {
			return fmt.Errorf("读取目录失败：%w", err)
		}
		if len(fsDict)-emptyFile < bf.saveDay {
			logging.Logger.Printf("%v有效备份数：%v,不足%v份", v, len(fsDict)-emptyFile, bf.saveDay)
		}
	}
	return nil
}

// 清理远端旧备份文件，传入远端机器路径，返回error
func (bf *backupFile) ClearRemote(dict string) error {
	//确认要保留的文件
	sshClient, err := bf.Connect()
	if err != nil {
		return err
	}
	defer sshClient.Close()

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	fsDict, err := sftpClient.ReadDir(dict)
	if err != nil {
		return fmt.Errorf("读取远程目录失败：%w", err)
	}

	for _, v := range fsDict {
		isContinue := false
		for index, dbName := range *bf.dbList {
			if index > len(*bf.dbList) || v.Name() == dbName {
				isContinue = false
				break
			}
			isContinue = true
		}

		if isContinue {
			continue
		}

		fsPath := dict + "/" + v.Name()

		fileList, err := sftpClient.ReadDir(fsPath)
		if err != nil {
			return fmt.Errorf("读取远程目录失败：%w", err)
		}
		cf := common.SortByTime(fileList)

		delDay := bf.saveDay
		if len(cf) < bf.saveDay {
			delDay = len(cf)
		}

		cf = cf[delDay:]

		//删除旧备份
		cmd := send.NewSftpOperater(sftpClient)
		for _, f := range cf {
			err := cmd.Remove(fsPath + "/" + f.Name())
			if err != nil {
				return fmt.Errorf("删除远程目录文件失败：%w", err)
			}
		}
	}

	return nil
}
