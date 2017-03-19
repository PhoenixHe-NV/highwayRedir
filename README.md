How to use it
---

Replace `<local_network>` and `<redirect_port`

```bash
git clone https://github.com/htc550605125/highwayRedir.git
cd highwayRedir/redir
go build
sudo cp redir /sbin
cd ..
sudo cp redir@.service /etc/systemd/system
sudo systemctl start reidr@<redirect_port>

sudo ip route add local 0.0.0.0/0 dev lo table 100
sudo ip rule add fwmark 0x1 lookup 100
sudo iptables -t mangle -A PREROUTING -p tcp -s <local_network> ! -d <local_network> -j TPROXY --on-port <redirect_port> --tproxy-mark 0x1

```
