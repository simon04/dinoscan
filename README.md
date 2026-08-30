# DinoScan

[Codex](https://doc.wikimedia.org/codex/latest/) frontend for [PetScan](https://petscan.wmcloud.org/).

## Deployment

The tool is deployed on [Toolforge](https://wikitech.wikimedia.org/wiki/Help:Toolforge/Deploy_your_tool)
at https://dinoscan.toolforge.org/ using the build service. `toolforge.yaml`
declares a single continuous component, built from this repository and published
at `/`; the `web` process in the `Procfile` serves the Vite build output via
`server.js`.

Register the configuration once, then deploy:

```sh
become dinoscan
toolforge components config create toolforge.yaml
toolforge components deployment create
```

Pushes to `master` deploy automatically once a deploy token
(`toolforge components deploy-token create`) is stored as the
`TOOLFORGE_DEPLOY_TOKEN` GitHub Actions secret.
