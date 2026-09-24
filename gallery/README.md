# Vigia routine gallery

Ready-made routines for [Vigia](https://github.com/denerFernandes/vigia). Each one is plain JavaScript you can read, with a manifest saying exactly what it may touch and tests that show what it does.

Vigia checks every routine before installing it:

1. The author is listed in `index.json` and the Ed25519 signature covers exactly this code, manifest and tests.
2. The routine was not removed after a report.
3. Its own tests pass.
4. An audit run with every capability reachable shows it calls only what the manifest declares.

If any check fails, Vigia refuses to install it and says why.

## Proposing a routine

1. In Vigia, open the routine and choose **Publicar na galeria**. You get a signed entry and your author key.
2. Add the routine as `routines/<id>.json` and your author entry to `index.json`, or run
   `vigia gallery build --key <your key file> --author <id> --name "Your Name" .`
3. Open a pull request. CI runs `vigia gallery verify index.json`, and a maintainer reads the code. Routines that send anything to other people (e-mail, WhatsApp to others) always get a human review.

## Reporting a routine

Open an issue with the **Report a routine** template. Confirmed reports are handled within 72 hours: the routine's hash goes into `revoked`, and every Vigia refuses it from then on, even if it is already installed elsewhere.
