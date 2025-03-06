# 秒杀服务

基于 mysql 和 redis 实现的秒杀服务，支持 10 万级别的并发

该服务定位为底层逻辑服务，通过 grpc 方式提供如下接口

## CreateSeckill

> 创建秒杀，返回秒杀 id

### 入参

```
message CreateSeckillRequest {
    string name = 1;
    string description = 2;
    int64 start_time = 3; // 开始时间
    int64 end_time = 4;   // 结束时间
    int64 total = 5;      // 库存数量
}
```

### 返回

```
message CreateSeckillResponse {
    int64 id = 1;
}
```

## JoinSeckill

> 加入秒杀

### 入参

```
message JoinSeckillRequest {
    int64 seckill_id = 1;
    int64 user_id = 2;
}
```

### 返回

```
enum JoinSeckillStatus {
    JOIN_UNKNOWN = 0;
    JOIN_NOT_START = 1; // 未开始
    JOIN_FINISHED = 2; // 已结束
    JOIN_NO_REMAINING = 3; // 已抢空
    JOIN_IN_USERS_LIST = 4; // 已在成功用户列表中
    JOIN_SUCCESS = 5; // 加入成功
    JOIN_FAILED = 6; // 加入失败
}
message JoinSeckillResponse {
    JoinSeckillStatus status = 1;
}
```

## InquireSeckill

> 查询秒杀状态，内部会阻塞，直到最终结果

### 入参

```
message InquireSeckillRequest {
    int64 seckill_id = 1;
    int64 user_id = 2;
}
```

### 返回

```
enum InquireSeckillStatus {
    IS_UNKNOWN = 0;
    IS_NOT_PARTICIPATING = 1; // 未参与
    IS_QUEUEING = 2; // 排队中
    IS_SUCCESS = 3; // 成功
    IS_FAILED = 4; // 失败
}
message InquireSeckillResponse {
    InquireSeckillStatus status = 1;
}
```
