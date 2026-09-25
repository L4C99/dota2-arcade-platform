package store

import (
	"context"
	"crypto/rand"
	"math/big"

	"github.com/jackc/pgx/v5"
)

// These names are presentation-only. User IDs remain the sole identity key.
var nameModifiers = []string{
	"薄荷", "琥珀", "晨雾", "月白", "松林", "青柠", "晚霞", "微风",
	"星河", "浅蓝", "暖阳", "白露", "清泉", "竹影", "云间", "初晴",
	"海盐", "杏花", "枫叶", "银杏", "雨后", "春山", "秋月", "冬晴",
	"星光", "麦田", "橘子", "栗色", "青竹", "雪松", "浅草", "碧波",
	"朝露", "落日", "花间", "晴空", "霜叶", "夏夜", "溪边", "山岚",
	"远山", "清晨", "蓝莓", "樱桃", "丁香", "云朵", "南风", "北星",
}

var nameNouns = []string{
	"水獭", "狐狸", "企鹅", "海豹", "猫头鹰", "鲸鱼", "白兔", "松鼠",
	"海豚", "小鹿", "云雀", "树懒", "刺猬", "天鹅", "海鸥", "浣熊",
	"熊猫", "羚羊", "喜鹊", "麻雀", "斑马", "河马", "海马", "袋鼠",
	"考拉", "鹦鹉", "小熊", "小象", "雪貂", "燕子", "鹭鸶", "山雀",
	"沙鸥", "花鹿", "野鸭", "青蛙", "蜜蜂", "蝴蝶", "萤火虫", "蒲公英",
	"向日葵", "小海星", "小贝壳", "红松果", "小竹笋", "小蘑菇", "小橘灯", "小风铃",
}

func newDisplayName() (string, error) {
	modifier, err := rand.Int(rand.Reader, big.NewInt(int64(len(nameModifiers))))
	if err != nil {
		return "", err
	}
	noun, err := rand.Int(rand.Reader, big.NewInt(int64(len(nameNouns))))
	if err != nil {
		return "", err
	}
	return nameModifiers[modifier.Int64()] + nameNouns[noun.Int64()], nil
}

func (s *Store) UserDisplayName(ctx context.Context, userID string) (string, error) {
	var name string
	err := s.Pool.QueryRow(ctx, `SELECT display_name FROM users WHERE id=$1`, userID).Scan(&name)
	return name, err
}

// Migration 9 backfill runs in the same transaction as its schema change.
// Existing non-null names are never rewritten, and duplicate names are valid.
func backfillUserDisplayNames(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `SELECT id FROM users WHERE display_name IS NULL ORDER BY id`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range ids {
		name, err := newDisplayName()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET display_name=$2 WHERE id=$1 AND display_name IS NULL`, id, name); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `ALTER TABLE users ALTER COLUMN display_name SET NOT NULL`)
	return err
}
