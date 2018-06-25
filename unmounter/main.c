#include <sys/mount.h>
#include <errno.h>

int main() {
  int result = umount("/ebs");

  // If the disk isn't mounted, return 0 because the
  // desired effect has already been achieved!
  if(result == 0 || errno == EINVAL) {
    return 0;
  } else {
    return 1;
  }        
}
