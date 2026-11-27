package errcode

// 外部业务号随单据生命周期保留，禁止通过删除草稿释放其身份。
var ExternalOrderProtected = NewHTTP(50014, "外部出库单不允许删除，请通过取消保留业务身份", 409)
