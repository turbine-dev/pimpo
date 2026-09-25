# Zodim routine gallery

Ready-made routines for [Zodim](https://github.com/denerFernandes/zodim). Each one is plain JavaScript you can read, with a manifest saying exactly what it may touch and tests that show what it does.

Zodim checks every routine before installing it:

1. The author is listed in `index.json` and the Ed25519 signature covers exactly this code, manifest and tests.
2. The routine was not removed after a report.
3. Its own tests pass.
4. An audit run with every capability reachable shows it calls only what the manifest declares.

If any check fails, Zodim refuses to install it and says why.

## Settings, not code

A routine declares what people may change in `manifest.params`: a city, a limit, some words, a currency. Types are text, number, boolean, date, time, location, select, multiselect, email and destinations. The code reads `params.<name>`, and people change the values in Zodim without touching the code. Messages to the owner go through `notify.send`, together with a `destinations` parameter, so each person picks where to be told. Tests can set `params` to show the routine works with values other than the defaults.

## Proposing a routine

1. In Zodim, open the routine and choose **Publicar na galeria**. You get a signed entry and your author key.
2. Add the routine as `routines/<id>.json` and your author entry to `index.json`, or run
   `zodim gallery build --key <your key file> --author <id> --name "Your Name" .`
3. Open a pull request. CI runs `zodim gallery verify index.json`, and a maintainer reads the code. Routines that send anything to other people (e-mail, WhatsApp to others) always get a human review.

## Reporting a routine

Open an issue with the **Report a routine** template. Confirmed reports are handled within 72 hours: the routine's hash goes into `revoked`, and every Zodim refuses it from then on, even if it is already installed elsewhere.
