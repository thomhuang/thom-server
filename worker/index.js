import { Container, getContainer } from "@cloudflare/containers";
import { env } from "cloudflare:workers";

// Runs the Go server (./Dockerfile) as a single Cloudflare Container.
// All application logic lives in Go; this shim only starts the container and
// forwards requests to it.

// Container env is a map of strings. A Worker secret that has not been set yet
// reads as undefined, and passing that through makes the container fail to
// start, so coerce everything to a string. The Go server treats "" as unset,
// which is the same as the variable being absent.
const envText = (value) =>
  value === undefined || value === null ? "" : String(value);

export class ThomServer extends Container {
  defaultPort = 4000;
  sleepAfter = "10m";
  pingEndpoint = "localhost/ping";

  // Worker secrets and vars are passed to the container on start.
  envVars = {
    ADMIN_USERNAME: envText(env.ADMIN_USERNAME),
    ADMIN_PASSWORD_HASH: envText(env.ADMIN_PASSWORD_HASH),
    JWT_SECRET: envText(env.JWT_SECRET),
    CLIENT_ORIGIN_URLS: envText(env.CLIENT_ORIGIN_URLS),
    SECURE_COOKIES: envText(env.SECURE_COOKIES),
    D1_ACCOUNT_ID: envText(env.D1_ACCOUNT_ID),
    D1_DATABASE_ID: envText(env.D1_DATABASE_ID),
    D1_ENDPOINT: envText(env.D1_ENDPOINT),
    R2_ENDPOINT: envText(env.R2_ENDPOINT),
    CF_API_TOKEN: envText(env.CF_API_TOKEN),
    R2_ACCOUNT_ID: envText(env.R2_ACCOUNT_ID),
    R2_BUCKET: envText(env.R2_BUCKET),
    R2_ACCESS_KEY_ID: envText(env.R2_ACCESS_KEY_ID),
    R2_SECRET_ACCESS_KEY: envText(env.R2_SECRET_ACCESS_KEY),
    R2_PUBLIC_BASE_URL: envText(env.R2_PUBLIC_BASE_URL),
    EMAIL_API_TOKEN: envText(env.EMAIL_API_TOKEN),
    EMAIL_ACCOUNT_ID: envText(env.EMAIL_ACCOUNT_ID),
    EMAIL_FROM: envText(env.EMAIL_FROM),
    EMAIL_FROM_NAME: envText(env.EMAIL_FROM_NAME),
    PUBLIC_SITE_URL: envText(env.PUBLIC_SITE_URL),
    ORDER_NOTIFICATION_EMAIL: envText(env.ORDER_NOTIFICATION_EMAIL),
    STRIPE_SECRET_KEY: envText(env.STRIPE_SECRET_KEY),
    STRIPE_WEBHOOK_SECRET: envText(env.STRIPE_WEBHOOK_SECRET),
    STRIPE_TAX_ENABLED: envText(env.STRIPE_TAX_ENABLED),
    STRIPE_SHIPPING_CENTS: envText(env.STRIPE_SHIPPING_CENTS),
  };
}

export default {
  async fetch(request, workerEnv) {
    // A single named instance keeps the server's in-memory login throttling
    // and token denylist consistent.
    return getContainer(workerEnv.THOM_SERVER).fetch(request);
  },
};
