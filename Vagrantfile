IP = ENV['NITROUS_INTERFACE'] || ENV['VAGRANT_INTERFACE'] || '33.33.33.12'

$script = <<SCRIPT
  export HOME=/home/vagrant

  apt-get update
  apt-get install -yy python-software-properties
  add-apt-repository http://ppa.launchpad.net/brightbox/experimental/ubuntu
  add-apt-repository ppa:ubuntu-lxc/stable

  apt-get update
  # install a newer kernel
  apt-get install -q -y linux-image-generic-lts-raring linux-headers-generic-lts-raring
  apt-get install -yy build-essential curl git-core bzr s3cmd mercurial
  curl -s https://get.docker.io/ubuntu/ | sudo sh

  # support for cgroup
  sed -i 's/GRUB_CMDLINE_LINUX=""/GRUB_CMDLINE_LINUX="cgroup_enable=memory swapaccount=1"/g' /etc/default/grub
  update-grub

  cd $HOME/nitrous
  source /etc/profile
SCRIPT

Vagrant.configure('2') do |config|
  config.vm.hostname = 'nitrous'

  if Vagrant::VERSION < '1.5.0'
    config.vm.box      = 'opscode_ubuntu-12.04_chef-11.2.0'
    config.vm.box_url  = 'https://opscode-vm.s3.amazonaws.com/vagrant/opscode_ubuntu-12.04_chef-11.2.0.box'
  else
    config.vm.box = 'hashicorp/precise64'
  end

  config.vm.network :private_network, ip: IP

  config.vm.synced_folder './', '/home/vagrant/nitrous'

  config.vm.provider :virtualbox do |v|
    v.customize ['modifyvm', :id, '--cpus', ENV['NITROUS_VM_CPU'] || '2']
    v.customize ['modifyvm', :id, '--memory', ENV['NITROUS_VM_MEM'] || '2048']
    v.customize ['modifyvm', :id, '--natdnshostresolver1', 'on']
    v.customize ['modifyvm', :id, '--natdnsproxy1', 'on']
  end

  config.ssh.forward_agent = true

  config.vm.provision 'shell', inline: $script
end
