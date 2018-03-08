docker build -t rootfsimage .
id=$(docker create rootfsimage true)
mkdir -p myplugin/rootfs
sudo docker export "$id" | tar -x -C myplugin/rootfs
docker rm -vf "$id"
docker rmi rootfsimage