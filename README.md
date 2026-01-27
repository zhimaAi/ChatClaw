# WillChat

## 开发

wails3 dev

## windows 打包

wails3 build

会生成二进制文件到 bin 目录，然后可以执行手动签名

## windows 分发

wails3 pacakge 

会生成安装包，对安装包也进行一次签名，即可分发

## MacOS 打包

### 打包arm64

wails3 task package ARCH=arm64

### 打包amd64

wails3 task package ARCH=amd64

### 打包通用二进制

wails3 task darwin:package:universal

## MacOS 分发

### 分发arm64

wails3 task darwin:create:dmg ARCH=arm64

### 分发amd64

wails3 task darwin:create:dmg ARCH=amd64

### 分发通用二进制

wails3 task darwin:create:dmg UNIVERSAL=true
