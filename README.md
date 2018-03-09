# SBLogDriver

SBLogDriver is a Docker log driver plugin that writes JSON logs to a shared mount.
* Captures stdin/stderr and write to file in JSON format
* Writes logs to shared mount on host (/mnt/sblogdriver/logs)
* Roll over logs at file size limit (--log-opt max-size=1m)
* Limits log file count by container (--log-opt max-file=5)
* Supports reading back of logs through `docker logs $container.id`
* Supports custom static data inclusion in logs via labels 

# Get Started
## Requirements
```
Ubuntu     ^16.04
Docker     ^17.0
Docker API ^1.26

User may need to be part of the docker group
usermod -aG docker <username>
```
## Build
```
# Build the plugin. Output is in myplugin/
sudo sh build.sh

ls myplugin
# config.json rootfs
```

## Install
```
# Install create and enable the plugin from myplugin/
sudo sh installplugin.sh

docker plugin ls
# ID               NAME                   DESCRIPTION                ENABLED
# eb57d2de3f20     sblogdriver:latest     jsonfile log as plugin     true
```

## Usage
```
# Start a detached container with SBLogDriver as the log driver
docker run -d --log-driver sblogdriver:latest [OPTIONS] <image_name>

# Logs will output to host at /mnt/sblogdriver/logs/<container.id>/application.log
# Logs will roll over to /mnt/sblogdriver/logs/<container.id>/application.log.1
```

## Uninstall
```
# First stop all containers using the plugin.

docker plugin disable sblogdriver:latest
docker plugin rm sblogdriver:latest
```

See [JSON file loggin driver](https://docs.docker.com/config/containers/logging/json-file/) for supported options related to the log driver.
