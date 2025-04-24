qemu-img resize base.qcow2 32G
qm create $vmid --name "$name-$vmid-cloudinit" --ostype l26 \
    --memory 1024 \
    --agent 1 \
    --bios ovmf --machine q35 --efidisk0 local-lvm:0,pre-enrolled-keys=0 \
    --cpu host --socket 1 --cores 2 \
    --vga serial0 --serial0 socket  \
    --net0 virtio,bridge=vmbr0
qm importdisk $vmid base.qcow2 local-lvm
qm set $vmid --scsihw virtio-scsi-pci --virtio0 local-lvm:vm-$vmid-disk-1,discard=on
qm set $vmid --boot order=virtio0
qm set $vmid --scsi1 local-lvm:cloudinit
qm set $vmid --ipconfig0 ip="dhcp"
qm set $vmid --tags $tags
qm set $vmid --cicustom "user=local:snippets/vendor.yaml"
qm template $vmid