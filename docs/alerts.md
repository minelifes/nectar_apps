# Alerts and push notifications

## Rules

Checked every 30 seconds (Alerts → Rules sets thresholds and durations):

- app or database down, crash loop, out-of-memory kills;
- node not ready, high CPU / memory, disk almost full, volume almost full;
- certificates about to expire, failed backups, an unreachable gateway, available updates;
- events: **deploy failed**, **deploy finished**.

Each alert notifies when it fires and when it clears; optional reminders repeat while it keeps firing.

## Channels

| Channel | |
|---|---|
| **ntfy** | your own ntfy server or ntfy.sh (pick a hard-to-guess topic); the ntfy app on your phone |
| **Telegram** | a bot from @BotFather and your chat id |
| **Email** | the panel's SMTP server (Settings → Email) or another one |
| **Webhook** | a JSON POST, signed with `X-Nectar-Signature: sha256=<HMAC>` when you set a secret |
| **Mobile app** | native push to the Nectar app (below) |

Every channel has a **Test** button.

## Native push notifications (optional)

The Nectar app on Android, iPhone/iPad and Mac can receive alerts as native notifications through **your own
Firebase project** (free tier). Tapping a notification opens the app or alert page.

1. Create a project at [console.firebase.google.com](https://console.firebase.google.com) and add an **Android
   app** and/or an **Apple app** with the Nectar app's package name / bundle id.
2. iPhone and Mac: create an APNs key in your Apple developer account and upload it under Firebase → Project
   settings → Cloud Messaging; enable the *Push Notifications* capability (and *Background Modes → Remote
   notifications* on iOS) in Xcode.
3. In the panel, **Settings → Push notifications → Set up**: paste the service account key (Project settings →
   Service accounts → Generate new private key; stored encrypted with the master key), the sender id, and the
   app ids and API keys of your apps.
4. Sign in to the Nectar app on your phone and allow notifications: it registers itself (Settings → Push
   notifications shows the number of devices; **Send test to my devices** checks the setup).
5. Add a **Mobile app** channel under Alerts. Leave *Users* empty to notify everyone who can see the server, or
   list the emails of the people to notify.

The app needs no `google-services.json` / `GoogleService-Info.plist`: it reads the Firebase options from the
panel after sign-in. Devices that uninstalled the app are forgotten automatically; signing out unregisters the
device.
