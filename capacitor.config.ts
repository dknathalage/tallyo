import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
  appId: 'com.invoices.app',
  appName: 'Invoice Manager',
  webDir: 'build',
  server: {
    androidScheme: 'https'
  }
};

export default config;
