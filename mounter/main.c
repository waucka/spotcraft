#include <sys/mount.h>
#include <stdio.h>

int main(int argc, char* argv[]) {
  if(argc < 2){
    fprintf(stderr, "USAGE: %s DEVICE\n", argv[0]);
    return 1;
  }

  int result = mount(argv[1], "/ebs", "ext4", MS_LAZYTIME | MS_NOEXEC | MS_NOSUID, "");

  if(result == 0) {
    return 0;
  } else {
    return 1;
  }        
}
