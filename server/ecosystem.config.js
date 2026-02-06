module.exports = {
  apps: [{
    name: 'nominatim-cache',
    script: 'dist/index.js',
    cwd: '/opt/nominatim-cache/server',
    instances: 1,
    exec_mode: 'fork',
    env: {
      NODE_ENV: 'production',
      PORT: 3000,
      DATABASE_PATH: './data/cache.db'
    },
    log_file: '/var/log/nominatim-cache/app.log',
    out_file: '/var/log/nominatim-cache/out.log',
    error_file: '/var/log/nominatim-cache/error.log',
    log_date_format: 'YYYY-MM-DD HH:mm:ss',
    max_memory_restart: '200M'
  }]
};
