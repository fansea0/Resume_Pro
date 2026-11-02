package interview

import "sort"

// GradeOrder 把等级映射为序数：C=0、B-=1、B=2、B+=3、A=4、S=5。
// 序数只用于排序与档位比较，不做加总平均；未知等级按 C 处理。
func GradeOrder(g Grade) int {
	switch g {
	case GradeC:
		return 0
	case GradeBMinus:
		return 1
	case GradeB:
		return 2
	case GradeBPlus:
		return 3
	case GradeA:
		return 4
	case GradeS:
		return 5
	}
	return 0
}

// GradeFromOrder 把序数映射回等级档位；越界序数按 C 处理。
func GradeFromOrder(o int) Grade {
	switch o {
	case 5:
		return GradeS
	case 4:
		return GradeA
	case 3:
		return GradeBPlus
	case 2:
		return GradeB
	case 1:
		return GradeBMinus
	}
	return GradeC
}

// ValidGrade 判断字符串是否为合法等级档位。
func ValidGrade(s string) bool {
	switch Grade(s) {
	case GradeC, GradeBMinus, GradeB, GradeBPlus, GradeA, GradeS:
		return true
	}
	return false
}

// IsGradeAtLeast 判断 g 是否不低于 threshold。
func IsGradeAtLeast(g, threshold Grade) bool { return GradeOrder(g) >= GradeOrder(threshold) }

// ClampGrade 取 parent 与 current 中的较高档位（只升不降）。
func ClampGrade(parent, current Grade) Grade {
	if GradeOrder(current) > GradeOrder(parent) {
		return current
	}
	return parent
}

// ComputeOverallGrade 计算整场面试的总评等级：中位数定基础档 + 比例门槛定高档。
//
// 规则（见 spec《评分与总评算法》）：
//   - 基础档：N 个序数排序后的中位数，偶数取下中位 orders[N/2-1]（保守，避免总评虚高）；
//   - S 档：基础档 ≥ A 且 S 题严格过半（sCount*2 > N）；
//   - A 档：基础档 ≥ B+ 且 A 及以上至少一半（aOrAbove*2 >= N）；
//   - B+ 档：基础档 ≥ B 且 B+ 及以上至少一半（bPlusOrAbove*2 >= N）；
//   - 都不命中则返回基础档。空输入返回 C。
func ComputeOverallGrade(grades []Grade) Grade {
	n := len(grades)
	if n == 0 {
		return GradeC
	}
	orders := make([]int, n)
	sCount, aOrAbove, bPlusOrAbove := 0, 0, 0
	for i, g := range grades {
		o := GradeOrder(g)
		orders[i] = o
		if o == 5 {
			sCount++
		}
		if o >= 4 {
			aOrAbove++
		}
		if o >= 3 {
			bPlusOrAbove++
		}
	}
	sort.Ints(orders)
	base := GradeFromOrder(medianOrder(orders))
	switch {
	case GradeOrder(base) >= GradeOrder(GradeA) && sCount*2 > n:
		return GradeS
	case GradeOrder(base) >= GradeOrder(GradeBPlus) && aOrAbove*2 >= n:
		return GradeA
	case GradeOrder(base) >= GradeOrder(GradeB) && bPlusOrAbove*2 >= n:
		return GradeBPlus
	}
	return base
}

// medianOrder 返回已排序序数切片的中位数下标：奇数取正中间，偶数取下中位
// （N/2-1，保守，避免总评虚高）。统一写法为 (len(orders)-1)/2。
func medianOrder(orders []int) int {
	return orders[(len(orders)-1)/2]
}
