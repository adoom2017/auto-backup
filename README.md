# auto-backup
Automatically compress and back up files by folder, with options to add a password and automatically upload to OneDrive or other cloud storage services.

## 使用方法
## Usage

### 直接执行
### Direct Execution
1. 下载代码
1. Download the code

2. 执行下面的命令进行编译`CGO_ENABLED=1 GOOS=linux go build -o auto-backup .`
2. Run the following command to compile: `CGO_ENABLED=1 GOOS=linux go build -o auto-backup .`

3. 修改`config_example.yaml`，按照配置文件说明进行配置
3. Modify `config_example.yaml` according to the configuration instructions

4. 将`config_example.yaml`文件重命名到`config.yaml`文件，需要和编译后的文件在同一目录下
4. Rename `config_example.yaml` to `config.yaml`, it needs to be in the same directory as the compiled file

5. 执行`./auto-backup`进行运行
5. Run `./auto-backup` to start the program

### docker执行
### Docker Execution
1. 生成docker镜像`docker build -t adoom2018/auto-backup:v1.0 .`
1. Build docker image: `docker build -t adoom2018/auto-backup:v1.0 .`

2. 运行镜像`docker run -d --name auto-backup -p 8080:8080 -v /path/to/backup:/root/backup -v /path/to/output:/root/output -v /path/to/config:/root/config -e CLIENT_ID=your_client_id -e CLIENT_SECRET=your_client_secret -e REDIRECT_URI=your_redirect_uri -e BACKUP_PASSWORD=your_password adoom2018/auto-backup:v1.0`
2. Run the image: `docker run -d --name auto-backup -p 8080:8080 -v /path/to/backup:/root/backup -v /path/to/output:/root/output -v /path/to/config:/root/config -e CLIENT_ID=your_client_id -e CLIENT_SECRET=your_client_secret -e REDIRECT_URI=your_redirect_uri -e BACKUP_PASSWORD=your_password adoom2018/auto-backup:v1.0`

### docker-compose执行
### Docker Compose Execution
1. 修改`docker-compose.yml`，按照配置文件说明进行配置
1. Modify `docker-compose.yml` according to the configuration instructions

2. 执行`docker-compose up -d`进行运行
2. Run `docker-compose up -d` to start

### 注意事项
### Notes
1. your_redirect_uri为添加Azure AD应用时设置的回调地址，需要和配置文件中的redirect_uri一致，一般设置为`http://localhost:8080/token`, 如果运行docker镜像的服务器上有https地址，那么可以直接使用该地址，就不用设置ssh隧道了
1. your_redirect_uri is the callback address set when adding the Azure AD application. It needs to match the redirect_uri in the configuration file, usually set to `http://localhost:8080/token`. If the server running the docker image has an https address, you can use that address directly without setting up an SSH tunnel.

2. 如果浏览器运行不是和docker或者程序在同一台机器上，那么需要在浏览器所在机器上设置localhost的host解析到docker所在机器的ip地址，或者使用ssh隧道
2. If the browser is not running on the same machine as docker or the program, you need to set up host resolution for localhost to the docker machine's IP address on the browser machine, or use an SSH tunnel:
    ```
    ssh -N -L 8080:remote_host:8080 user@remote_host
    ```

---
> 如果是第一次启动，需要查看日志，将日志打印的连接复制到浏览器中，进行认证
>
> For first-time startup, check the logs and copy the printed URL to your browser for authentication

> 程序会自动压缩root_dir目录下的文件，并输出到output_dir目录下，并且会自动上传到OneDrive
> 
>The program will automatically compress files in the root_dir directory, output them to the output_dir directory, and automatically upload them to OneDrive

> 后续每天增量备份文件，即只有新增或者修改的文件才会被备份
>
> Subsequent daily incremental backups will only backup new or modified files

> 如果需要强制全量备份，可以设置force_full_backup为true
>
> If you need to force a full backup, set force_full_backup to true

> 如果需要修改备份时间，可以修改cron的值，cron的值为cron表达式，可以参考https://pkg.go.dev/github.com/robfig/cron
>
> If you need to modify the backup schedule, you can change the cron value. The cron value is a cron expression, refer to https://pkg.go.dev/github.com/robfig/cron