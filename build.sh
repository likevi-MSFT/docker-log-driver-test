docker build -t rootfsimage .
id=$(docker create rootfsimage true)
mkdir -p myplugin/rootfs
sudo docker export "$id" | tar -x -C myplugin/rootfs
cp config.json myplugin/config.json
mkdir -p myplugin/rootfs/run/docker/plugins # needed to communicate with Docker, this is where the jsonfile.sock will be
docker rm -vf "$id"
docker rmi rootfsimage