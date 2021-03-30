### Linux

```shell
curl -L https://github.com/jenkins-x/jx-pipeline/releases/download/v{{.Version}}/jx-pipeline-linux-amd64.tar.gz | tar xzv 
sudo mv jx-pipeline /usr/local/bin
```

### macOS

```shell
curl -L  https://github.com/jenkins-x/jx-pipeline/releases/download/v{{.Version}}/jx-pipeline-darwin-amd64.tar.gz | tar xzv
sudo mv jx-pipeline /usr/local/bin
```

