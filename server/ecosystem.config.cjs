module.exports = {
  apps: [{
    name: 'nominatim-cache',
    script: 'dist/index.js',
    cwd: process.env.NOMINATIM_CACHE_SERVER_CWD || '/opt/nominatim-cache/server',
    instances: 1,
    exec_mode: 'fork',
    env: {
      NODE_ENV: 'production'
    },
    log_file: './logs/app.log',
    out_file: './logs/out.log',
    error_file: './logs/error.log',
    log_date_format: 'YYYY-MM-DD HH:mm:ss',
    max_memory_restart: '200M'
  }]
};
