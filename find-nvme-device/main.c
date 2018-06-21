#include <unistd.h>
#include <stdio.h>

int main(int argc, char* argv[]) {
  if(argc < 2){
    fprintf(stderr, "USAGE: %s VOLUMEID\n", argv[0]);
    return 1;
  }

  char* exec_args[] = {"/usr/bin/find-nvme-device.sh", argv[1], NULL};
  execv("/usr/bin/find-nvme-device.sh", exec_args);

  return 1;
}
