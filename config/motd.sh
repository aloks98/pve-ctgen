#!/bin/bash
echo -e "\033[1;34m"
figlet -f standard "e412.in" | lolcat
echo -e "\033[1;32m"
echo "Welcome to $(hostname)!"
echo "OS: $(lsb_release -ds)"
echo "Kernel: $(uname -r)"
echo "CPU: $(lscpu | grep "Model name" | cut -f 2 -d ":" | sed "s/^[ \t]*//")"
echo "Memory: $(free -h | grep Mem | awk '{print $2}')"
echo "IP: $(hostname -I | awk '{print $1}')"
echo -e "\033[1;36m"
echo "Random Quote of the Day:"
fortune | cowsay -f $(ls /usr/share/cowsay/cows/ | shuf -n1)
echo -e "\033[0m" 