# Test
由于cigno完全基于本地数据处理（tarball process）和远端API交互（image registry API v2），
所以在测试时，必须提供相应的环境用于交互，即需要提供registry。

建议的方案是，基于`distribution`，提供容器化的registry服务，用于测试使用。