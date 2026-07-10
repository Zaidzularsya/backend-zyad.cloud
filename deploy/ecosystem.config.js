module.exports = {
  apps: [
    {
      name: 'zyad-api',
      cwd: '/var/www/zyad-cloud-backend/current',
      script: './api',
      interpreter: 'none',
      env: { APP_ENV: 'production' },
    },
    {
      name: 'zyad-worker',
      cwd: '/var/www/zyad-cloud-backend/current',
      script: './worker',
      interpreter: 'none',
      env: { APP_ENV: 'production' },
    },
  ],
}
