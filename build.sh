docker build -t rootfsimage .
id=$(docker create rootfsimage true)
sudo mkdir -p myplugin/rootfs
sudo mkdir -p myplugin/rootfs
docker rm -vf "$id"
docker rmi rootfsimage