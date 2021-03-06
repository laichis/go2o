/**
 * Copyright 2014 @ z3q.net.
 * name :
 * author : jarryliu
 * date : 2013-12-09 10:13
 * description :
 * history :
 */

package repository

import (
	"database/sql"
	"fmt"
	"github.com/jsix/gof"
	"github.com/jsix/gof/db"
	"github.com/jsix/gof/db/orm"
	"github.com/jsix/gof/storage"
	"go2o/core"
	"go2o/core/domain/interface/member"
	"go2o/core/domain/interface/mss"
	"go2o/core/domain/interface/valueobject"
	memberImpl "go2o/core/domain/member"
	"go2o/core/dto"
	"go2o/core/infrastructure/format"
	"go2o/core/infrastructure/tool"
	"go2o/core/variable"
	"log"
	"strings"
	"sync"
	"time"
)

var _ member.IMemberRepo = new(MemberRepo)
var (
	memberManager member.IMemberManager
	memberMux     sync.Mutex
)

type MemberRepo struct {
	Storage storage.Interface
	db.Connector
	_orm     orm.Orm
	_valRepo valueobject.IValueRepo
	_mssRepo mss.IMssRepo
}

func NewMemberRepo(sto storage.Interface, c db.Connector, mssRepo mss.IMssRepo,
	valRepo valueobject.IValueRepo) *MemberRepo {
	return &MemberRepo{
		Storage:   sto,
		Connector: c,
		_orm:      c.GetOrm(),
		_mssRepo:  mssRepo,
		_valRepo:  valRepo,
	}
}

// 获取管理服务
func (m *MemberRepo) GetManager() member.IMemberManager {
	memberMux.Lock()
	if memberManager == nil {
		memberManager = memberImpl.NewMemberManager(m, m._valRepo)
	}
	memberMux.Unlock()
	return memberManager
}

// 获取资料或初始化
func (m *MemberRepo) GetProfile(memberId int64) *member.Profile {
	e := &member.Profile{}
	key := m.getProfileCk(memberId)
	if m.Storage.Get(key, &e) != nil {
		if err := m.Connector.GetOrm().Get(memberId, e); err != nil {
			if err == sql.ErrNoRows {
				e.MemberId = memberId
				orm.Save(m.GetOrm(), e, 0)
			}
		} else {
			m.Storage.Set(key, *e)
		}
	}
	return e
}

// 保存资料
func (m *MemberRepo) SaveProfile(v *member.Profile) error {
	_, _, err := m.Connector.GetOrm().Save(v.MemberId, v)
	if err == nil {
		err = m.Storage.Set(m.getProfileCk(v.MemberId), *v)
	}
	return err
}

//收藏,typeId 为类型编号, referId为关联的ID
func (m *MemberRepo) Favorite(memberId int64, favType int, referId int32) error {
	_, _, err := m.Connector.GetOrm().Save(nil, &member.Favorite{
		MemberId:   memberId,
		FavType:    favType,
		ReferId:    referId,
		UpdateTime: time.Now().Unix(),
	})
	return err
}

//是否已收藏
func (m *MemberRepo) Favored(memberId int64, favType int, referId int32) bool {
	num := 0
	m.Connector.ExecScalar(`SELECT COUNT(0) FROM mm_favorite
	WHERE member_id=? AND fav_type=? AND refer_id=?`, &num,
		memberId, favType, referId)
	return num > 0
}

//取消收藏
func (m *MemberRepo) CancelFavorite(memberId int64, favType int, referId int32) error {
	_, err := m.Connector.GetOrm().Delete(&member.Favorite{},
		"member_id=? AND fav_type=? AND refer_id=?",
		memberId, favType, referId)
	return err
}

var (
	globLevels []*member.Level
)

// 获取会员等级
func (m *MemberRepo) GetMemberLevels_New() []*member.Level {
	const key = "go2o:rep:level:glob:cache"
	i, err := m.Storage.GetInt(key)
	load := err != nil || i != 1 || globLevels == nil
	if load {
		list := []*member.Level{}
		m.Connector.GetOrm().Select(&list, "1=1 ORDER BY id ASC")
		globLevels = list
		m.Storage.Set(key, 1)
	}
	return globLevels
}

// 获取等级对应的会员数
func (m *MemberRepo) GetMemberNumByLevel_New(id int32) int {
	total := 0
	m.Connector.ExecScalar("SELECT COUNT(0) FROM mm_member WHERE level=?", &total, id)
	return total
}

// 删除会员等级
func (m *MemberRepo) DeleteMemberLevel_New(id int32) error {
	err := m.Connector.GetOrm().DeleteByPk(&member.Level{}, id)
	if err == nil {
		PrefixDel(m.Storage, "go2o:rep:level:*")
	}
	return err
}

// 保存会员等级
func (m *MemberRepo) SaveMemberLevel_New(v *member.Level) (int32, error) {
	id, err := orm.I32(orm.Save(m.GetOrm(), v, int(v.ID)))
	if err == nil {
		PrefixDel(m.Storage, "go2o:rep:level:*")
	}
	return id, err
}

// 根据用户名获取会员
func (m *MemberRepo) GetMemberByUsr(usr string) *member.Member {
	e := &member.Member{}
	err := m.Connector.GetOrm().GetBy(e, "usr=?", usr)
	if err == nil {
		return e
	}
	return nil
}

// 根据手机号码获取会员
func (m *MemberRepo) GetMemberValueByPhone(phone string) *member.Member {
	e := &member.Member{}
	err := m.GetOrm().GetByQuery(e, `SELECT * FROM mm_member
		INNER JOIN mm_profile ON mm_profile.member_id = mm_member.id
		 WHERE phone=?`, phone)
	if err != nil {
		return nil
	}
	return e
}

// 根据手机号获取会员编号
func (m *MemberRepo) GetMemberIdByPhone(phone string) int64 {
	var id int64
	m.Connector.ExecScalar("SELECT member_id FROM mm_profile WHERE phone=?", &id, phone)
	return id
}

// 根据邮箱地址获取会员编号
func (m *MemberRepo) GetMemberIdByEmail(email string) int64 {
	var id int64
	m.Connector.ExecScalar("SELECT member_id FROM mm_profile WHERE email=?", &id, email)
	return id
}

func (m *MemberRepo) getMemberCk(memberId int64) string {
	return fmt.Sprintf("go2o:rep:mm:inf:%d", memberId)
}
func (m *MemberRepo) getAccountCk(memberId int64) string {
	return fmt.Sprintf("go2o:rep:mm:%d:acc", memberId)
}
func (m *MemberRepo) getProfileCk(memberId int64) string {
	return fmt.Sprintf("go2o:rep:mm:pro:%d", memberId)
}
func (m *MemberRepo) getTrustCk(memberId int64) string {
	return fmt.Sprintf("go2o:rep:mm:trust:%d", memberId)
}
func (m *MemberRepo) getGlobLevelsCk() string {
	return "go2o:rep:mm-lv"
}

// 获取会员
func (m *MemberRepo) GetMember(memberId int64) member.IMember {
	e := &member.Member{}
	key := m.getMemberCk(memberId)
	if err := m.Storage.Get(key, &e); err != nil {
		//log.Println("-- mm",err)
		if m.Connector.GetOrm().Get(memberId, e) != nil {
			return nil
		}
		m.Storage.Set(key, *e)
	} else {
		//log.Println(fmt.Sprintf("--- member: %d > %#v",memberId,e))
	}
	return m.CreateMember(e)
}

// 保存会员
func (m *MemberRepo) SaveMember(v *member.Member) (int64, error) {
	if v.Id > 0 {
		rc := core.GetRedisConn()
		defer rc.Close()
		// 保存最后更新时间
		mutKey := fmt.Sprintf("%s%d", variable.KvMemberUpdateTime, v.Id)
		rc.Do("SETEX", mutKey, 3600*400, v.UpdateTime)
		rc.Do("RPUSH", variable.KvMemberUpdateTcpNotifyQueue, v.Id) // push to tcp notify queue

		// 保存会员信息
		_, _, err := m.Connector.GetOrm().Save(v.Id, v)

		if err == nil {
			// 存储到缓存中
			err = m.Storage.Set(m.getMemberCk(v.Id), *v)
			// 存储到队列
			rc.Do("RPUSH", variable.KvMemberUpdateQueue, fmt.Sprintf("%d-update", v.Id))
		}
		return v.Id, err
	}

	return m.createMember(v)
}

func (m *MemberRepo) createMember(v *member.Member) (int64, error) {
	var id int64
	_, id, err := m.Connector.GetOrm().Save(nil, v)
	if err != nil {
		return -1, err
	}
	v.Id = id
	m.initMember(v)

	rc := core.GetRedisConn()
	defer rc.Close()
	rc.Do("RPUSH", variable.KvMemberUpdateQueue,
		fmt.Sprintf("%d-create", v.Id)) // push to queue

	// 更新会员数 todo: 考虑去掉
	var total = 0
	m.Connector.ExecScalar("SELECT COUNT(0) FROM mm_member", &total)
	gof.CurrentApp.Storage().Set(variable.KvTotalMembers, total)

	return v.Id, err
}

func (m *MemberRepo) initMember(v *member.Member) {
	orm := m.Connector.GetOrm()
	orm.Save(nil, &member.Account{
		MemberId:     v.Id,
		Balance:      0,
		TotalExpense: 0,
		TotalCharge:  0,
		TotalPay:     0,
		UpdateTime:   v.RegTime,
	})

	orm.Save(nil, &member.BankInfo{
		MemberId: v.Id,
		State:    1,
	})

	orm.Save(nil, &member.Relation{
		MemberId:  v.Id,
		CardCard:  "",
		InviterId: 0,
		RegMchId:  0,
	})
}

// 删除会员
func (m *MemberRepo) DeleteMember(id int64) error {
	m.Storage.Del(m.getMemberCk(id))
	_, err := m.ExecNonQuery("delete from mm_member where id = ?", id)
	sql := `
    /* 清理会员 */
     delete from mm_profile where member_id NOT IN (select id from mm_member) and member_id > 0;
     delete from mm_bank where member_id NOT IN(SELECT id FROM mm_member) and member_id > 0;
     delete from mm_account where member_id NOT IN(SELECT id FROM mm_member) and member_id > 0;
     delete from mm_relation where member_id NOT IN(SELECT id FROM mm_member) and member_id > 0;
     delete from mm_integral_log where member_id NOT IN (SELECT id FROM mm_member) and id > 0;
     delete from pay_order where buy_user NOT IN(SELECT id FROM mm_member) and id > 0;
     delete from mm_levelup where member_id NOT IN(SELECT id FROM mm_member) and id > 0;
    `
	for _, v := range strings.Split(sql, ";") {
		if v = strings.TrimSpace(v); len(v) > 5 {
			_, err := m.ExecNonQuery(v)
			if err != nil {
				log.Println("执行清理出错:", err, " sql:", v)
			}
		}
	}
	return err
}

func (m *MemberRepo) GetMemberIdByUser(user string) int64 {
	var id int64
	m.Connector.ExecScalar("SELECT id FROM mm_member WHERE usr = ?", &id, user)
	return id
}

// 创建会员
func (m *MemberRepo) CreateMember(v *member.Member) member.IMember {
	return memberImpl.NewMember(m.GetManager(), v, m,
		m._mssRepo, m._valRepo)
}

// 创建会员,仅作为某些操作使用,不保存
func (m *MemberRepo) CreateMemberById(memberId int64) member.IMember {
	return m.CreateMember(&member.Member{Id: memberId})
}

// 根据邀请码获取会员编号
func (m *MemberRepo) GetMemberIdByInvitationCode(code string) int64 {
	var memberId int64
	m.ExecScalar("SELECT id FROM mm_member WHERE invitation_code=?", &memberId, code)
	return memberId
}

// 获取会员最后更新时间
func (m *MemberRepo) GetMemberLatestUpdateTime(memberId int64) int64 {
	var updateTime int64
	m.Connector.ExecScalar(`SELECT update_time FROM mm_member where id=?`,
		&updateTime, memberId)
	return updateTime
}

// 获取账户
func (m *MemberRepo) GetAccount(memberId int64) *member.Account {
	e := &member.Account{}
	key := m.getAccountCk(memberId)
	if m.Storage.Get(key, &e) != nil {
		if m.Connector.GetOrm().Get(memberId, e) != nil {
			return nil
		}
		m.Storage.Set(key, *e)
	} else {
		//log.Println(key,fmt.Sprintf("--- account: %d > %#v",memberId,e))
	}
	return e
}

// 保存账户，传入会员编号
func (m *MemberRepo) SaveAccount(v *member.Account) (int64, error) {
	_, _, err := m.Connector.GetOrm().Save(v.MemberId, v)
	if err == nil {
		m.pushToAccountUpdateQueue(v.MemberId, v.UpdateTime)
		m.Storage.Set(m.getAccountCk(v.MemberId), *v)
	}
	return v.MemberId, err
}

func (m *MemberRepo) pushToAccountUpdateQueue(memberId int64, updateTime int64) {
	rc := core.GetRedisConn()
	defer rc.Close()
	// 保存最后更新时间
	mutKey := fmt.Sprintf("%s%d", variable.KvAccountUpdateTime, memberId)
	rc.Do("SETEX", mutKey, 3600*400, updateTime)
	// push to tcp notify queue
	rc.Do("RPUSH", variable.KvAccountUpdateTcpNotifyQueue, memberId)
}

// 获取银行信息
func (m *MemberRepo) GetBankInfo(memberId int64) *member.BankInfo {
	e := new(member.BankInfo)
	m.Connector.GetOrm().Get(memberId, e)
	return e
}

// 保存银行信息
func (m *MemberRepo) SaveBankInfo(v *member.BankInfo) error {
	var err error
	_, _, err = m.Connector.GetOrm().Save(v.MemberId, v)
	return err
}

// 保存积分记录
func (m *MemberRepo) SaveIntegralLog(v *member.IntegralLog) error {
	_, err := orm.Save(m.GetOrm(), v, int(v.Id))
	return err
}

// 保存余额日志
func (m *MemberRepo) SaveBalanceLog(v *member.BalanceLog) (int32, error) {
	return orm.I32(orm.Save(m.GetOrm(), v, int(v.Id)))
}

// 保存钱包账户日志
func (m *MemberRepo) SavePresentLog(v *member.WalletLog) (int32, error) {
	return orm.I32(orm.Save(m.GetOrm(), v, int(v.ID)))
}

func (m *MemberRepo) GetWalletLog(id int32) *member.WalletLog {
	e := member.WalletLog{}
	if err := m.Connector.GetOrm().Get(id, &e); err != nil {
		return nil
	}
	return &e
}

// 获取会员提现次数键
func (m *MemberRepo) getMemberTakeOutTimesKey(memberId int64) string {
	return fmt.Sprintf("sys:go2o:rep:mm:take-out-times:%d", memberId)
}

// 增加会员当天提现次数
func (m *MemberRepo) AddTodayTakeOutTimes(memberId int64) error {
	times := m.GetTodayTakeOutTimes(memberId)
	key := m.getMemberTakeOutTimesKey(memberId)
	// 保存到当天结束
	t := time.Now()
	d := (24-t.Hour())*3600 + (60-t.Minute())*60 + (60 - t.Second())
	return m.Storage.SetExpire(key, times+1, int64(d))
}

// 获取会员每日提现次数
func (m *MemberRepo) GetTodayTakeOutTimes(memberId int64) int {
	key := m.getMemberTakeOutTimesKey(memberId)
	applyTimes, _ := m.Storage.GetInt(key)
	return applyTimes

	total := 0
	b, e := tool.GetStartEndUnix(time.Now())
	err := m.ExecScalar(`SELECT COUNT(0) FROM mm_wallet_log WHERE
        member_id=? AND kind IN(?,?) AND create_time BETWEEN ? AND ?`, &total,
		memberId, member.KindWalletTakeOutToBankCard,
		member.KindWalletTakeOutToThirdPart, b, e)
	if err != nil {
		handleError(err)
	}
	return total
}

func (m *MemberRepo) getRelationCk(memberId int64) string {
	return fmt.Sprintf("go2o:rep:mm:%d:rel", memberId)
}

// 获取会员关联
func (m *MemberRepo) GetRelation(memberId int64) *member.Relation {
	e := member.Relation{}
	key := m.getRelationCk(memberId)
	if m.Storage.Get(key, &e) != nil {
		if err := m.Connector.GetOrm().Get(memberId, &e); err != nil {
			return nil
		}
		m.Storage.Set(key, e)
	}
	return &e
}

// 获取积分对应的等级
func (m *MemberRepo) GetLevelValueByExp(mchId int32, exp int64) int {
	var levelId int
	m.Connector.ExecScalar(`SELECT lv.value FROM pt_member_level lv
	 	where lv.merchant_id=? AND lv.require_exp <= ? AND lv.enabled=1
	 	 ORDER BY lv.require_exp DESC LIMIT 0,1`,
		&levelId, mchId, exp)
	return levelId

}

// 用户名是否存在
func (m *MemberRepo) CheckUsrExist(usr string, memberId int64) bool {
	var c int
	m.Connector.ExecScalar("SELECT COUNT(0) FROM mm_member WHERE usr=? AND id<>?",
		&c, usr, memberId)
	return c != 0
}

// 手机号码是否使用
func (m *MemberRepo) CheckPhoneBind(phone string, memberId int64) bool {
	var c int
	m.Connector.ExecScalar("SELECT COUNT(0) FROM mm_profile WHERE phone=? AND member_id<>?",
		&c, phone, memberId)
	return c != 0
}

// 保存绑定
func (m *MemberRepo) SaveRelation(v *member.Relation) error {
	_, _, err := m.Connector.GetOrm().Save(v.MemberId, v)
	if err == nil {
		err = m.Storage.Set(m.getRelationCk(v.MemberId), *v)
	}
	return err
}

// 获取会员升级记录
func (m *MemberRepo) GetLevelUpLog(id int32) *member.LevelUpLog {
	e := member.LevelUpLog{}
	if m.GetOrm().Get(id, &e) == nil {
		return &e
	}
	return nil
}

// 保存会员升级记录
func (m *MemberRepo) SaveLevelUpLog(v *member.LevelUpLog) (int32, error) {
	return orm.I32(orm.Save(m.GetOrm(), v, int(v.Id)))
}

// 保存地址
func (m *MemberRepo) SaveDeliver(v *member.Address) (int64, error) {
	return orm.I64(orm.Save(m.Connector.GetOrm(), v, int(v.ID)))
}

// 获取全部配送地址
func (m *MemberRepo) GetDeliverAddress(memberId int64) []*member.Address {
	addresses := []*member.Address{}
	m.Connector.GetOrm().Select(&addresses, "member_id=?", memberId)
	return addresses
}

// 获取配送地址
func (m *MemberRepo) GetSingleDeliverAddress(memberId, deliverId int64) *member.Address {
	var address member.Address
	err := m.Connector.GetOrm().Get(deliverId, &address)

	if err == nil && address.MemberId == memberId {
		return &address
	}
	return nil
}

// 删除配送地址
func (m *MemberRepo) DeleteAddress(memberId, deliverId int64) error {
	_, err := m.Connector.ExecNonQuery(
		"DELETE FROM mm_deliver_addr WHERE member_id=? AND id=?",
		memberId, deliverId)
	return err
}

// 邀请
func (m *MemberRepo) GetMyInvitationMembers(memberId int64, begin, end int) (
	total int, rows []*dto.InvitationMember) {
	arr := []*dto.InvitationMember{}
	m.Connector.ExecScalar(`SELECT COUNT(0) FROM mm_member WHERE id IN
	 (SELECT member_id FROM mm_relation WHERE inviter_id=?)`, &total, memberId)
	if total > 0 {
		m.Connector.Query(`SELECT m.id,m.usr,m.level,p.avatar,p.name,p.phone,p.im FROM
            (SELECT id,usr,level FROM mm_member WHERE id IN (SELECT member_id FROM
             mm_relation WHERE inviter_id=?) ORDER BY level DESC,id LIMIT ?,?) m
             INNER JOIN mm_profile p ON p.member_id = m.id ORDER BY level DESC,id`,
			func(rs *sql.Rows) {
				for rs.Next() {
					e := &dto.InvitationMember{}
					rs.Scan(&e.MemberId, &e.User, &e.Level, &e.Avatar, &e.NickName, &e.Phone, &e.Im)
					arr = append(arr, e)
				}
			}, memberId, begin, end-begin)
	}
	return total, arr
}

// 获取下级会员数量
func (m *MemberRepo) GetSubInvitationNum(memberId int64, memberIdArr []int32) map[int32]int {
	if len(memberIdArr) == 0 {
		return map[int32]int{}
	}
	memberIds := format.IdArrJoinStr32(memberIdArr)
	var d map[int32]int = make(map[int32]int)
	err := m.Connector.Query(fmt.Sprintf("SELECT r1.member_id,"+
		"(SELECT COUNT(0) FROM mm_relation r2 WHERE r2.inviter_id=r1.member_id)"+
		"as num FROM mm_relation r1 WHERE r1.member_id IN(%s)", memberIds),
		func(rows *sql.Rows) {
			var id int32
			var num int
			for rows.Next() {
				rows.Scan(&id, &num)
				d[id] = num
			}
		})
	handleError(err)
	return d
}

// 获取推荐我的人
func (m *MemberRepo) GetInvitationMeMember(memberId int64) *member.Member {
	var d *member.Member = new(member.Member)
	err := m.Connector.GetOrm().GetByQuery(d,
		"SELECT * FROM mm_member WHERE id =(SELECT inviter_id FROM mm_relation  WHERE id=?)",
		memberId)

	if err != nil {
		return nil
	}
	return d
}

// 根据编号获取余额变动信息
func (m *MemberRepo) GetBalanceInfo(id int32) *member.BalanceInfo {
	var e member.BalanceInfo
	if err := m.Connector.GetOrm().Get(id, &e); err == nil {
		return &e
	}
	return nil
}

// 根据号码获取余额变动信息
func (m *MemberRepo) GetBalanceInfoByNo(tradeNo string) *member.BalanceInfo {
	var e member.BalanceInfo
	if err := m.Connector.GetOrm().GetBy(&e, "trade_no=?", tradeNo); err == nil {
		return &e
	}
	return nil
}

// 保存余额变动信息
func (m *MemberRepo) SaveBalanceInfo(v *member.BalanceInfo) (int32, error) {
	return orm.I32(orm.Save(m.GetOrm(), v, int(v.Id)))
}

// 保存理财账户信息
func (m *MemberRepo) SaveGrowAccount(memberId int64, balance, totalAmount,
	growEarnings, totalGrowEarnings float32, updateTime int64) error {
	_, err := m.Connector.ExecNonQuery(`UPDATE mm_account SET grow_balance=?,
		grow_amount=?,grow_earnings=?,grow_total_earnings=?,update_time=? where member_id=?`,
		balance, totalAmount, growEarnings, totalGrowEarnings, updateTime, memberId)
	//清除缓存
	m.Storage.Del(m.getAccountCk(memberId))
	//加入通知队列
	m.pushToAccountUpdateQueue(memberId, updateTime)
	return err
}

// 获取会员分页的优惠券列表
func (m *MemberRepo) GetMemberPagedCoupon(memberId int64, start, end int, where string) (total int, rows []*dto.SimpleCoupon) {
	list := []*dto.SimpleCoupon{}
	m.Connector.ExecScalar(fmt.Sprintf(`SELECT COUNT(distinct pi.id)
        FROM pm_info pi INNER JOIN pm_coupon c ON c.id = pi.id
	    INNER JOIN pm_coupon_bind pb ON pb.coupon_id=pi.id
	    WHERE member_id=? AND %s`, where), &total, memberId)
	if total > 0 {
		m.Connector.GetOrm().SelectByQuery(&list,
			fmt.Sprintf(`SELECT pi.id,SUM(1) as num,pi.short_name as title,
            code,fee,c.discount,is_used,over_time FROM pm_info pi
             INNER JOIN pm_coupon c ON c.id = pi.id
	        INNER JOIN pm_coupon_bind pb ON pb.coupon_id=pi.id
	        WHERE member_id=? AND %s GROUP BY pi.id order by bind_time DESC LIMIT ?,?`, where),
			memberId, start, end-start)
	}
	return total, list
}

// Select MmBuyerGroup
func (m *MemberRepo) SelectMmBuyerGroup(where string, v ...interface{}) []*member.BuyerGroup {
	list := []*member.BuyerGroup{}
	err := m._orm.Select(&list, where, v...)
	if err != nil && err != sql.ErrNoRows {
		log.Println("[ Orm][ Error]:", err.Error(), "; Entity:MmBuyerGroup")
	}
	return list
}

// Save MmBuyerGroup
func (m *MemberRepo) SaveMmBuyerGroup(v *member.BuyerGroup) (int, error) {
	id, err := orm.Save(m._orm, v, int(v.ID))
	if err != nil && err != sql.ErrNoRows {
		log.Println("[ Orm][ Error]:", err.Error(), "; Entity:MmBuyerGroup")
	}
	return id, err
}
