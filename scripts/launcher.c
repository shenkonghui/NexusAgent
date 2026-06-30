/*
 * NexusAgent macOS 启动器包装器
 *
 * macOS 的 .app bundle 要求 CFBundleExecutable 是一个 Mach-O 可执行文件，
 * 不能直接执行 shell 脚本。这个小 wrapper 编译为二进制后作为 CFBundleExecutable，
 * 它会调用同目录下的 launcher.sh 脚本。
 *
 * 编译: clang -o launcher launcher.c
 */
#include <stdlib.h>
#include <unistd.h>
#include <string.h>
#include <stdio.h>
#include <mach-o/dyld.h>
#include <libgen.h>

int main(int argc, char *argv[]) {
    char exe_path[4096];
    uint32_t size = sizeof(exe_path);
    if (_NSGetExecutablePath(exe_path, &size) != 0) {
        return 1;
    }

    /* 取 exe 所在目录，拼接 launcher.sh */
    char *dir = dirname(exe_path);
    char script_path[4200];
    snprintf(script_path, sizeof(script_path), "%s/launcher.sh", dir);

    /* exec shell 脚本，替换当前进程 */
    char *new_argv[] = { script_path, NULL };
    execvp(script_path, new_argv);

    /* 如果 exec 失败，回退到默认 shell */
    char *fallback_argv[] = { "/bin/bash", script_path, NULL };
    execvp("/bin/bash", fallback_argv);

    return 1;
}
