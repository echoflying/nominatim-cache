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
      DATABASE_PATH: './data/cache.db',
      ADMIN_USERNAME: 'change-me',
      ADMIN_PASSWORD: 'change-me-too'
    },
    log_file: './logs/app.log',
    out_file: './logs/out.log',
    error_file: './logs/error.log',
    log_date_format: 'YYYY-MM-DD HH:mm:ss',
    max_memory_restart: '200M'
  }]
};
